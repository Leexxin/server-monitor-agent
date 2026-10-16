using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.NetworkInformation;
using System.Net.Sockets;
using System.ServiceProcess;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using Microsoft.Win32;

namespace SmaManager {
    internal sealed class AgentStatus {
        public string State = "未安装", Startup = "未知", Version = "未知", Listen = "", Error = "";
        public Dictionary<string, object> Snapshot;
    }
    internal static class AgentClient {
        public static readonly string Root = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "SMA");
        public static readonly string Data = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "SMA");
        public static readonly string PowerShell = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System), @"WindowsPowerShell\v1.0\powershell.exe");
        public static JavaScriptSerializer Json { get { return new JavaScriptSerializer { MaxJsonLength = 2 * 1024 * 1024 }; } }
        public static Dictionary<string, object> Object(object value) { return value as Dictionary<string, object> ?? new Dictionary<string, object>(); }
        public static object Get(Dictionary<string, object> map, string name) { object value; return map.TryGetValue(name, out value) ? value : null; }
        public static string Text(Dictionary<string, object> map, string name) { return Convert.ToString(Get(map, name), CultureInfo.InvariantCulture); }
        public static double Number(Dictionary<string, object> map, string name) { object value = Get(map,name); return value == null ? 0 : Convert.ToDouble(value, CultureInfo.InvariantCulture); }
        public static Dictionary<string, object> Config() { return Json.Deserialize<Dictionary<string, object>>(File.ReadAllText(Path.Combine(Data,"config.json"))); }

        // Quote Windows argv, never interpolate user data into PowerShell source.
        public static string Quote(string value) {
            var result = new StringBuilder("\""); int slashes = 0;
            foreach (char ch in value) {
                if (ch == '\\') { slashes++; continue; }
                if (ch == '"') { result.Append('\\', slashes * 2 + 1); result.Append(ch); slashes = 0; continue; }
                result.Append('\\', slashes); slashes = 0; result.Append(ch);
            }
            result.Append('\\', slashes * 2); return result.Append('"').ToString();
        }
        public static string Run(string executable, params string[] args) {
            var info = new ProcessStartInfo(executable, string.Join(" ", args.Select(Quote))) {
                UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true, RedirectStandardError = true,
                StandardOutputEncoding = Encoding.UTF8, StandardErrorEncoding = Encoding.UTF8, WorkingDirectory = Path.GetTempPath()
            };
            using (var process = Process.Start(info)) {
                Task<string> output = process.StandardOutput.ReadToEndAsync(), error = process.StandardError.ReadToEndAsync();
                if (!process.WaitForExit(90000)) { try { process.Kill(); } catch { } throw new InvalidOperationException("操作超时，请查看日志并确认服务状态。"); }
                Task.WaitAll(output,error);
                string text = output.Result + error.Result;
                if (process.ExitCode != 0) throw new InvalidOperationException(Redact(text));
                return text.Trim();
            }
        }
        public static string Ctl(params string[] args) { return Run(Path.Combine(Root,"sma-ctl.exe"),args); }
        public static string Script(string name, params string[] args) {
            var list = new List<string> { "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", Path.Combine(Root,name) };
            list.AddRange(args); return Run(PowerShell,list.ToArray());
        }
        public static string Redact(string text) {
            text = Regex.Replace(text ?? "", @"(?i)\b[0-9a-f]{64}\b", "[凭据已隐藏]");
            return Regex.Replace(text, @"(?im)(authorization\s*[:=]\s*bearer\s+|token\s*[:=]\s*)([^\s]+)", "$1[已隐藏]");
        }
        public static string Token() { return Ctl("token","show"); }
        public static List<string> Addresses() {
            var result = new List<string>();
            foreach (var adapter in NetworkInterface.GetAllNetworkInterfaces()) {
                if (adapter.OperationalStatus != OperationalStatus.Up) continue;
                foreach (var address in adapter.GetIPProperties().UnicastAddresses) {
                    if (address.Address.AddressFamily == AddressFamily.InterNetwork && !IPAddress.IsLoopback(address.Address)) result.Add(address.Address.ToString());
                }
            }
            return result.Distinct().OrderBy(x => x.StartsWith("169.254.") ? 1 : 0).ToList();
        }
        public static Uri LocalEndpoint(string listen) {
            Uri uri;
            if (!Uri.TryCreate("http://"+listen,UriKind.Absolute,out uri)) throw new InvalidOperationException("监听地址无效。");
            IPAddress ip;
            if (!IPAddress.TryParse(uri.Host.Trim('[',']'),out ip)) throw new InvalidOperationException("监听地址必须是 IP 地址。");
            string host = ip.Equals(IPAddress.Any) ? "127.0.0.1" : ip.Equals(IPAddress.IPv6Any) ? "[::1]" : uri.Host;
            return new Uri("http://"+host+":"+uri.Port);
        }
        public static Dictionary<string,object> Fetch(Dictionary<string,object> config) {
            var request = (HttpWebRequest)WebRequest.Create(new Uri(LocalEndpoint(Text(config,"listenAddress")),"/v1/snapshot"));
            request.Proxy = null; request.AllowAutoRedirect = false; request.Timeout = 5000; request.ReadWriteTimeout = 5000;
            request.Headers[HttpRequestHeader.Authorization] = "Bearer " + Token();
            using (var response = (HttpWebResponse)request.GetResponse()) {
                if (response.StatusCode != HttpStatusCode.OK) throw new InvalidOperationException("快照接口返回 HTTP "+(int)response.StatusCode);
                using (var reader = new StreamReader(response.GetResponseStream())) {
                    char[] data = new char[1024 * 1024 + 1]; int count = 0, n;
                    while (count < data.Length && (n=reader.Read(data,count,data.Length-count))>0) count+=n;
                    if (count >= data.Length) throw new InvalidOperationException("快照响应过大。");
                    return Json.Deserialize<Dictionary<string,object>>(new string(data,0,count));
                }
            }
        }
        public static AgentStatus Status() {
            var status = new AgentStatus();
            try {
                using (var service = new ServiceController("SMA")) {
                    status.State = service.Status == ServiceControllerStatus.Running ? "运行中" : service.Status == ServiceControllerStatus.Stopped ? "已停止" : "正在切换状态";
                }
                using (var key = Registry.LocalMachine.OpenSubKey(@"SYSTEM\CurrentControlSet\Services\SMA")) status.Startup = Convert.ToInt32(key.GetValue("Start",3)) == 2 ? "已启用" : "未启用";
                using (var key = Registry.LocalMachine.OpenSubKey(@"SOFTWARE\SMA")) if (key!=null) status.Version = Convert.ToString(key.GetValue("Version","未知"));
                var config = Config(); status.Listen = Text(config,"listenAddress");
                if (status.State == "运行中") status.Snapshot = Fetch(config);
            } catch (Exception error) { status.Error = Redact(error.Message); }
            return status;
        }
        public static string LogTail() {
            string path = Path.Combine(Data,@"logs\sma.log");
            if (!File.Exists(path)) return "还没有运行日志。";
            using (var stream = new FileStream(path,FileMode.Open,FileAccess.Read,FileShare.ReadWrite | FileShare.Delete)) {
                if (stream.Length > 65536) stream.Seek(-65536,SeekOrigin.End);
                using (var reader = new StreamReader(stream)) return Redact(reader.ReadToEnd());
            }
        }
        public static string Diagnostics() {
            string doctor;
            try { doctor = Ctl("doctor"); } catch (Exception error) { doctor=error.Message; }
            var s = Status();
            return Redact("SMA 诊断报告\r\n时间："+DateTime.Now.ToString("s")+"\r\n版本："+s.Version+"\r\n服务："+s.State+"\r\n开机自启："+s.Startup+"\r\n监听："+s.Listen+"\r\n错误："+s.Error+"\r\n\r\n"+doctor+"\r\n\r\n最近运行日志\r\n"+LogTail());
        }
    }
}
