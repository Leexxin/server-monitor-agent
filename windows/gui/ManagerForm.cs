using System;
using System.Collections;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace SmaManager {
    internal sealed class ManagerForm : Form {
        readonly Color Ink=Color.FromArgb(25,39,47), Muted=Color.FromArgb(84,104,115), Accent=Color.FromArgb(0,113,105);
        readonly TabControl tabs = new TabControl { Dock=DockStyle.Fill, Name="Sections" };
        readonly Label state = new Label(), banner = new Label(), footer = new Label(), meta = new Label();
        readonly Label cpu = new Label(), memory = new Label(), disk = new Label(), facts = new Label();
        readonly TextBox url = new TextBox(), listen = new TextBox(), sources = new TextBox(), token = new TextBox(), output = new TextBox();
        readonly ComboBox host = new ComboBox();
        readonly Button startup = new Button();
        readonly ProgressBar progress = new ProgressBar { Dock=DockStyle.Bottom, Height=4, Visible=false, Style=ProgressBarStyle.Marquee };
        readonly Timer refresh = new Timer { Interval=4000 }, privacy = new Timer { Interval=1000 };
        bool busy, loadedConfig;
        string currentListen="", copiedToken="";
        DateTime tokenUntil, copyUntil;
        AgentStatus last;
        double previousTotal=-1, previousIdle;
        public string TestDirectory;

        public ManagerForm() {
            Text="SMA 管理中心"; Name="SmaManager"; Font=new Font("Microsoft YaHei UI",9F);
            ForeColor=Ink; BackColor=Color.White; StartPosition=FormStartPosition.CenterScreen;
            AutoScaleDimensions=new SizeF(96,96); AutoScaleMode=AutoScaleMode.Dpi;
            ClientSize=new Size(980,720); MinimumSize=new Size(860,640);
            var area=Screen.PrimaryScreen.WorkingArea; Size=new Size(Math.Min(Width,area.Width-24),Math.Min(Height,area.Height-24));
            var top=new Panel { Dock=DockStyle.Top,Height=96,BackColor=Color.FromArgb(239,246,246) };
            var brand=new Label { Text="SMA",Font=new Font(Font.FontFamily,25,FontStyle.Bold),AutoSize=true,Location=new Point(24,12) };
            top.Controls.Add(brand);
            top.Controls.Add(new Label {Text="本机 Agent 管理中心",AutoSize=true,ForeColor=Muted,Location=new Point(132,27)});
            state.AutoSize=true; state.Font=new Font(Font,FontStyle.Bold); state.Location=new Point(24,65); top.Controls.Add(state);
            meta.AutoSize=true; meta.Location=new Point(170,65); meta.ForeColor=Muted; top.Controls.Add(meta);
            banner.Dock=DockStyle.Top; banner.Height=54; banner.Padding=new Padding(24,12,24,6); banner.BackColor=Color.FromArgb(255,248,225); banner.Text="正在检查安装结果与服务状态…";
            var bottom=new Panel { Dock=DockStyle.Bottom,Height=46,Padding=new Padding(20,10,20,4) };
            footer.Dock=DockStyle.Fill; footer.ForeColor=Muted; footer.Text="关闭此窗口后，SMA 服务仍在后台运行。"; bottom.Controls.Add(footer);
            BuildOverview(); BuildConnection(); BuildMaintenance(); BuildDiagnostics();
            Controls.Add(tabs); Controls.Add(banner); Controls.Add(top); Controls.Add(progress); Controls.Add(bottom);
            refresh.Tick+=async delegate { if(!busy) await RefreshStatus(); };
            privacy.Tick+=delegate {
                if(token.Text.Length>0 && DateTime.UtcNow>tokenUntil) HideToken();
                if(copiedToken.Length>0 && DateTime.UtcNow>copyUntil) ClearOwnedClipboard();
            };
            Shown+=async delegate {
                try {
                    await RefreshStatus(); await LoadConnection(); privacy.Start();
                    if(TestDirectory!=null) { await SelfTest(); return; }
                    refresh.Start();
                } catch(Exception error) {
                    if(TestDirectory==null){Error(error);return;}
                    Directory.CreateDirectory(TestDirectory);File.WriteAllText(Path.Combine(TestDirectory,"gui-fatal.txt"),AgentClient.Redact(error.ToString()));Environment.ExitCode=1;busy=false;Close();
                }
            };
            FormClosing+=delegate(object sender,FormClosingEventArgs e) { if(busy) { e.Cancel=true; footer.Text="当前操作仍在进行，请等待结果后关闭窗口。"; } };
            FormClosed+=delegate { refresh.Stop(); privacy.Stop(); ClearOwnedClipboard(); HideToken(); };
        }
        TableLayoutPanel Page(string title) {
            var page=new TabPage(title) { BackColor=Color.White,Padding=new Padding(20),AutoScroll=true };
            var stack=new TableLayoutPanel { Dock=DockStyle.Top,AutoSize=true,AutoSizeMode=AutoSizeMode.GrowAndShrink,ColumnCount=1,Padding=new Padding(0,2,0,12) };
            stack.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100)); page.Controls.Add(stack); tabs.TabPages.Add(page); return stack;
        }
        void Add(TableLayoutPanel stack,Control control,int height) {
            int row=stack.RowCount++; stack.RowStyles.Add(new RowStyle(SizeType.Absolute,height)); control.Dock=DockStyle.Fill; control.Margin=new Padding(0,0,0,12); stack.Controls.Add(control,0,row);
        }
        Label Note(string text) { return new Label { Text=text,ForeColor=Muted,AutoEllipsis=true,Padding=new Padding(0,4,0,0) }; }
        Button Button(string text,Func<Task> action,bool primary=false) {
            var button=new Button { Text=text,AutoSize=true,MinimumSize=new Size(110,36),Height=36,Padding=new Padding(10,3,10,3),FlatStyle=FlatStyle.Flat,Margin=new Padding(0,0,10,8),BackColor=primary?Accent:Color.White,ForeColor=primary?Color.White:Ink,AccessibleName=text };
            button.FlatAppearance.BorderColor=primary?Accent:Color.FromArgb(197,210,214);
            button.Click+=async delegate { await action(); }; return button;
        }
        FlowLayoutPanel Actions(params Control[] controls) {
            var row=new FlowLayoutPanel { Dock=DockStyle.Fill,WrapContents=true,AutoScroll=false }; row.Controls.AddRange(controls); return row;
        }
        Panel Card(string title,Label value,string caption) {
            var card=new Panel { Dock=DockStyle.Fill,BackColor=Color.FromArgb(242,247,248),Padding=new Padding(16),Margin=new Padding(0,0,12,0) };
            var a=new Label {Text=title,Dock=DockStyle.Top,Height=25,ForeColor=Muted};
            value.Dock=DockStyle.Fill; value.Font=new Font(Font.FontFamily,24,FontStyle.Bold); value.Text="—"; value.TextAlign=ContentAlignment.MiddleLeft;
            var c=new Label {Text=caption,Dock=DockStyle.Bottom,Height=25,ForeColor=Muted}; card.Controls.Add(value);card.Controls.Add(c);card.Controls.Add(a); return card;
        }
        void BuildOverview() {
            var page=Page("运行总览");
            Add(page,new Label {Text="安装后，从这里确认 SMA 是否正常工作",Font=new Font(Font.FontFamily,15,FontStyle.Bold)},44);
            var cards=new TableLayoutPanel { ColumnCount=3,RowCount=1 };
            for(int i=0;i<3;i++) cards.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,33.333F));
            cards.Controls.Add(Card("CPU 使用率",cpu,"两次累计采样计算"),0,0);cards.Controls.Add(Card("内存使用率",memory,"物理内存"),1,0);cards.Controls.Add(Card("系统盘使用率",disk,"Windows 系统卷"),2,0);
            Add(page,cards,146); facts.ForeColor=Muted; Add(page,facts,60);
            Add(page,new Label {Text="接入 sma-center",Font=new Font(Font,FontStyle.Bold)},30);
            Add(page,Note("1. 选择调用方能够访问的本机 IP。  2. 复制 Agent URL。  3. 到“连接与 Token”复制凭据。"),36);
            host.DropDownStyle=ComboBoxStyle.DropDownList;host.Width=210; host.AccessibleName="用于对接的本机 IP";
            host.SelectedIndexChanged+=delegate { UpdateUrl(); };
            Add(page,Actions(host,Button("刷新本机 IP",delegate { LoadAddresses(); return Task.FromResult(0); })),48);
            url.ReadOnly=true;url.Font=new Font("Consolas",12);url.AccessibleName="Agent URL";Add(page,url,42);
            Add(page,Actions(Button("复制 Agent URL",delegate { if(url.Text.Length>0) Clipboard.SetText(url.Text); footer.Text="已复制 Agent URL。";return Task.FromResult(0); },true),Button("配置连接与 Token",delegate {tabs.SelectedIndex=1;return Task.FromResult(0);}),Button("重新检查",RefreshStatus)),52);
            Add(page,Note("仅本机监听时，请先开启远程监听并限定 sma-center 来源 IP。本机检测通过后，再从调用方验证网络连通。"),52);
            LoadAddresses();
        }
        void BuildConnection() {
            var page=Page("连接与 Token");
            Add(page,new Label {Text="连接配置",Font=new Font(Font.FontFamily,15,FontStyle.Bold)},40);
            Add(page,Note("监听地址（IP:端口）"),26); listen.AccessibleName="监听地址";Add(page,listen,38);
            Add(page,Actions(Button("仅本机访问",delegate {listen.Text="127.0.0.1:"+Port();return Task.FromResult(0);}),Button("允许远程接入",delegate {listen.Text="0.0.0.0:"+Port();sources.Focus();return Task.FromResult(0);})),48);
            Add(page,Note("允许访问的 sma-center IP 或 CIDR（多项用英文逗号分隔，例如 192.168.1.10）"),30);
            sources.AccessibleName="防火墙允许来源";Add(page,sources,38);
            Add(page,Actions(Button("保存连接设置",SaveConnection,true),Button("重新读取配置",LoadConnection)),48);
            Add(page,Note("保存将重启服务并应用 SMA 防火墙规则。远程接入必须填写来源；其他防火墙规则仍可能影响实际可达性。"),42);
            Add(page,new Label {Text="Bearer Token",Font=new Font(Font.FontFamily,14,FontStyle.Bold)},38);
            token.ReadOnly=true;token.UseSystemPasswordChar=true;token.AccessibleName="Bearer Token（默认隐藏）";token.Font=new Font("Consolas",10);Add(page,token,36);
            Add(page,Actions(Button("显示 30 秒",ShowToken),Button("复制 Token",CopyToken,true),Button("立即隐藏",delegate {HideToken();return Task.FromResult(0);}),Button("重置 Token…",RotateToken)),52);
            Add(page,Note("将原始 Token 填入 sma-center，不带 Bearer 前缀。复制后 60 秒内，若剪贴板仍是本次 Token，将自动清空。重置会使旧凭据失效。"),50);
        }
        void BuildMaintenance() {
            var page=Page("服务与维护");
            Add(page,new Label {Text="后台服务",Font=new Font(Font.FontFamily,15,FontStyle.Bold)},44);
            Add(page,Actions(Button("启动服务",delegate {return Service("start");},true),Button("停止服务…",async delegate {if(Confirm("停止后，sma-center 将暂时无法获取指标。继续停止？"))await Service("stop");}),Button("重启服务",delegate {return Service("restart");})),54);
            startup.Text="检查开机自启"; startup.AutoSize=true;startup.MinimumSize=new Size(160,36); startup.Click+=async delegate {await Service(last!=null&&last.Startup=="已启用"?"disable":"enable");}; Add(page,Actions(startup),52);
            Add(page,Note("服务独立于管理界面运行。关闭本窗口不影响采集，也不需要保持用户登录。"),48);
            Add(page,new Label {Text="升级与卸载",Font=new Font(Font.FontFamily,14,FontStyle.Bold)},40);
            Add(page,Actions(Button("选择新版 MSI 升级…",Upgrade),Button("卸载 SMA…",delegate {return Uninstall(false);}),Button("卸载并清除数据…",delegate {return Uninstall(true);})),54);
            Add(page,Note("升级保留配置与 Token。默认卸载保留数据；“清除数据”会永久删除默认配置、Token 和运行日志。维护开始后本窗口会关闭，并显示独立进度和结果窗口。"),68);
            Add(page,Actions(Button("打开安装目录",delegate {OpenFolder(AgentClient.Root);return Task.FromResult(0);}),Button("打开数据目录",delegate {OpenFolder(AgentClient.Data);return Task.FromResult(0);}),Button("查看操作指南",Help)),52);
            Add(page,Note("首次安装由 MSI 完成。若界面提示未安装，请先运行正式 MSI。Server Core 无桌面时继续使用 sma-ctl 和安装脚本。"),54);
        }
        void BuildDiagnostics() {
            var page=Page("日志与诊断");
            Add(page,Actions(Button("运行诊断",Diagnose,true),Button("刷新运行日志",async delegate {await Work("正在读取日志…",async delegate {output.Text=await Task.Run(()=>AgentClient.LogTail());});}),Button("导出诊断报告…",Export),Button("打开日志目录",delegate {OpenFolder(Path.Combine(AgentClient.Data,"logs"));return Task.FromResult(0);})),52);
            Add(page,Note("报告包含服务状态、监听地址和最近运行日志，不主动包含配置备份或 Token；导出前仍可在下方检查内容。"),46);
            output.Multiline=true;output.ReadOnly=true;output.ScrollBars=ScrollBars.Both;output.WordWrap=false;output.Font=new Font("Consolas",9);output.BackColor=Color.FromArgb(246,248,249);output.AccessibleName="日志与诊断输出";Add(page,output,340);
        }
        void LoadAddresses() {
            string old=Convert.ToString(host.SelectedItem);host.Items.Clear();
            foreach(string address in AgentClient.Addresses())host.Items.Add(address);
            if(host.Items.Count==0)host.Items.Add("127.0.0.1");
            host.SelectedIndex=host.Items.Contains(old)?host.Items.IndexOf(old):0;
        }
        int Port() { Uri uri;return Uri.TryCreate("http://"+listen.Text,UriKind.Absolute,out uri)?uri.Port:9108; }
        void UpdateUrl() {
            if(currentListen.Length==0){url.Text="";return;}
            try {
                var uri=new Uri("http://"+currentListen);var ip=IPAddress.Parse(uri.Host.Trim('[',']'));
                string selected=ip.Equals(IPAddress.Any)||ip.Equals(IPAddress.IPv6Any)?Convert.ToString(host.SelectedItem):uri.Host;
                if(selected.Contains(":"))selected="["+selected.Trim('[',']')+"]";
                url.Text="http://"+selected+":"+uri.Port;
            }catch{url.Text="";}
        }
        async Task RefreshStatus() {
            await Work("正在检查服务与指标…",async delegate {
                last=await Task.Run(()=>AgentClient.Status()); currentListen=last.Listen;
                state.Text="● "+last.State;state.ForeColor=last.State=="运行中"&&last.Error.Length==0?Accent:Color.FromArgb(160,82,15);
                meta.Text="版本 "+last.Version+"    开机自启："+last.Startup+"    检查于 "+DateTime.Now.ToString("HH:mm:ss");
                startup.Text=last.Startup=="已启用"?"关闭开机自启":"启用开机自启";
                UpdateUrl();
                if(!loadedConfig && last.Listen.Length>0){listen.Text=last.Listen;loadedConfig=true;}
                if(last.Error.Length>0){banner.Text="需要处理："+last.Error;cpu.Text=memory.Text=disk.Text="—";return;}
                if(last.State!="运行中"){banner.Text="服务尚未运行，请到“服务与维护”启动服务。";cpu.Text=memory.Text=disk.Text="—";previousTotal=-1;return;}
                bool local=last.Listen.StartsWith("127.")||last.Listen.StartsWith("[::1]");
                banner.Text=local?"本机检测通过。当前仅允许本机访问；远程接入请打开“连接与 Token”。":"本机服务和鉴权正常。将 Agent URL 与 Token 填入 sma-center 完成接入。";
                if(last.Snapshot==null)return;
                var s=last.Snapshot;var c=AgentClient.Object(AgentClient.Get(s,"cpu"));
                double total=0,idle=0;
                foreach(var item in AgentClient.Object(AgentClient.Get(c,"seconds"))) {
                    var modes=AgentClient.Object(item.Value);total+=modes.Sum(x=>Convert.ToDouble(x.Value));idle+=AgentClient.Number(modes,"idle");
                }
                cpu.Text=previousTotal>=0&&total>previousTotal&&idle>=previousIdle?(Math.Max(0,Math.Min(100,100*(1-(idle-previousIdle)/(total-previousTotal))))).ToString("0.0")+"%":"采样中";
                previousTotal=total;previousIdle=idle;
                var m=AgentClient.Object(AgentClient.Get(s,"memory"));double memTotal=AgentClient.Number(m,"totalBytes"),available=AgentClient.Number(m,"availableBytes");
                memory.Text=memTotal>0?(100*(1-available/memTotal)).ToString("0")+"%":"—";
                var machine=AgentClient.Object(AgentClient.Get(s,"host"));string mount=AgentClient.Text(machine,"systemMountpoint");
                disk.Text="—";var volumes=AgentClient.Get(s,"filesystems") as IEnumerable;
                if(volumes!=null)foreach(var entry in volumes){var fs=AgentClient.Object(entry);if(AgentClient.Text(fs,"mountpoint").Equals(mount,StringComparison.OrdinalIgnoreCase)){double size=AgentClient.Number(fs,"sizeBytes");if(size>0)disk.Text=(100*(1-AgentClient.Number(fs,"availableBytes")/size)).ToString("0")+"%";}}
                var uptime=TimeSpan.FromSeconds(AgentClient.Number(s,"uptimeSeconds"));
                facts.Text=AgentClient.Text(machine,"hostname")+"    "+AgentClient.Number(c,"logicalCount")+" 个逻辑 CPU    内存 "+(memTotal/1073741824).ToString("0.0")+" GiB\r\n运行时间 "+uptime.Days+"天 "+uptime.Hours+"小时 "+uptime.Minutes+"分钟    系统盘 "+mount;
                var warnings=AgentClient.Get(s,"errors") as IEnumerable;
                if(warnings!=null&&warnings.Cast<object>().Any())banner.Text="服务可用，但部分指标采集有警告。请到“日志与诊断”查看详情。";
            });
        }
        async Task LoadConnection() {
            await Work("正在读取连接配置…",async delegate {
                var config=await Task.Run(()=>AgentClient.Config());listen.Text=AgentClient.Text(config,"listenAddress");
                string raw=await Task.Run(()=>AgentClient.Script("configure-network.ps1","-Read"));
                var firewall=AgentClient.Json.Deserialize<Dictionary<string,object>>(raw);var list=AgentClient.Get(firewall,"sources") as IEnumerable;
                sources.Text=list==null?"":string.Join(",",list.Cast<object>().Select(Convert.ToString));loadedConfig=true;
            });
        }
        async Task SaveConnection() {
            string address=listen.Text.Trim(),allowed=sources.Text.Trim();
            bool saved=await Work("正在保存连接并检查服务，请稍候…",async delegate {
                string result=await Task.Run(()=>AgentClient.Script("configure-network.ps1","-ListenAddress",address,"-Sources",allowed));
                output.Text=AgentClient.Redact(result);footer.Text="连接配置已保存。";
            });
            if(saved)await RefreshStatus();
        }
        async Task ShowToken() {
            await Work("正在读取凭据…",async delegate {string value=await Task.Run(()=>AgentClient.Token());token.Text=value;token.UseSystemPasswordChar=false;tokenUntil=DateTime.UtcNow.AddSeconds(30);footer.Text="Token 已显示，30 秒后自动隐藏。";});
        }
        async Task CopyToken() {
            await Work("正在复制凭据…",async delegate {copiedToken=await Task.Run(()=>AgentClient.Token());Clipboard.SetText(copiedToken);copyUntil=DateTime.UtcNow.AddSeconds(60);footer.Text="Token 已复制。请粘贴到 sma-center；60 秒后按归属清空剪贴板。";});
        }
        void HideToken(){token.Text="";token.UseSystemPasswordChar=true;}
        void ClearOwnedClipboard(){try{if(copiedToken.Length>0&&Clipboard.ContainsText()&&Clipboard.GetText()==copiedToken)Clipboard.Clear();}catch{}copiedToken="";}
        async Task RotateToken(){if(!Confirm("重置 Token 会立即使 sma-center 等客户端的旧凭据失效。确认重置？"))return;HideToken();ClearOwnedClipboard();bool done=await Work("正在重置凭据…",async delegate{output.Text=await Task.Run(()=>AgentClient.Ctl("token","reset","--yes"));footer.Text="Token 已重置，请复制新值并更新 sma-center。";});if(done)await RefreshStatus();}
        async Task Service(string action){bool done=await Work("正在变更服务，请等待结果…",async delegate{output.Text=await Task.Run(()=>AgentClient.Ctl(action));});if(done)await RefreshStatus();}
        async Task Diagnose(){await Work("正在生成诊断报告…",async delegate{output.Text=await Task.Run(()=>AgentClient.Diagnostics());});}
        async Task Export(){await Diagnose();using(var dialog=new SaveFileDialog {Filter="文本报告 (*.txt)|*.txt",FileName="sma-diagnostics-"+DateTime.Now.ToString("yyyyMMdd-HHmmss")+".txt"}){if(dialog.ShowDialog(this)==DialogResult.OK){try{File.WriteAllText(dialog.FileName,AgentClient.Redact(output.Text),new UTF8Encoding(true));footer.Text="诊断报告已导出。";}catch(Exception e){Error(e);}}}}
        Task Help(){using(var dialog=new Form {Text="SMA 操作指南",Size=new Size(850,650),StartPosition=FormStartPosition.CenterParent}){var text=new TextBox{Multiline=true,ReadOnly=true,ScrollBars=ScrollBars.Vertical,Dock=DockStyle.Fill,Font=Font};string path=Path.Combine(AgentClient.Root,@"docs\windows\README.md");text.Text=File.Exists(path)?File.ReadAllText(path):"请查看安装包中的 docs/windows/README.md。";dialog.Controls.Add(text);dialog.ShowDialog(this);}return Task.FromResult(0);}
        Task Upgrade(){using(var dialog=new OpenFileDialog{Filter="SMA 安装包 (*.msi)|*.msi",CheckFileExists=true}){if(dialog.ShowDialog(this)!=DialogResult.OK)return Task.FromResult(0);try{string version=Maintenance.VerifyMsi(dialog.FileName);if(Confirm("将使用选中的 SMA MSI 更新到 "+version+"。配置与 Token 将保留。\r\n升级期间采集会短暂停止，继续？")){Maintenance.Launch("upgrade",dialog.FileName);Close();}}catch(Exception e){Error(e);}}return Task.FromResult(0);}
        Task Uninstall(bool purge){if(Confirm(purge?"永久删除默认配置、Token 和运行日志，并卸载 SMA？此操作不能撤销，请先备份。":"卸载 SMA 服务和程序？配置、Token 和运行日志将保留。")){try{Maintenance.Launch(purge?"purge":"uninstall","");Close();}catch(Exception e){Error(e);}}return Task.FromResult(0);}
        bool Confirm(string message){return MessageBox.Show(this,message,"请确认",MessageBoxButtons.YesNo,MessageBoxIcon.Warning,MessageBoxDefaultButton.Button2)==DialogResult.Yes;}
        void OpenFolder(string path){try{if(!Directory.Exists(path))throw new DirectoryNotFoundException("目录尚不存在："+path);System.Diagnostics.Process.Start("explorer.exe",AgentClient.Quote(path));}catch(Exception e){Error(e);}}
        void Error(Exception e){string text=AgentClient.Redact(e.Message);banner.Text="操作未完成："+text;footer.Text="请到日志与诊断查看详情。";MessageBox.Show(this,text,"操作未完成",MessageBoxButtons.OK,MessageBoxIcon.Error);}
        async Task<bool> Work(string message,Func<Task> action){if(busy)return false;busy=true;tabs.Enabled=false;progress.Visible=true;footer.Text=message;try{await action();if(footer.Text==message)footer.Text="操作完成 · "+DateTime.Now.ToString("HH:mm:ss");return true;}catch(Exception e){if(TestDirectory!=null)throw;Error(e);return false;}finally{busy=false;tabs.Enabled=true;progress.Visible=false;}}

        async Task SelfTest(){
            try {
                await Task.Delay(1200);await RefreshStatus();
                if(last==null||last.Snapshot==null||last.Error.Length>0)throw new Exception("GUI native snapshot verification failed");
                string secret=await Task.Run(()=>AgentClient.Token());
                if(token.Text.Length!=0||!token.UseSystemPasswordChar)throw new Exception("Token must be hidden by default");
                if(AgentClient.Redact(secret).Contains(secret))throw new Exception("Secret redaction failed");
                if(url.Text.Length==0||meta.Text.Length==0||listen.Text.Length==0)throw new Exception("Missing connection or state controls");
                Directory.CreateDirectory(TestDirectory);
                for(int i=0;i<tabs.TabPages.Count;i++){
                    tabs.SelectedIndex=i;PerformLayout();Application.DoEvents();
                    using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(0,0,Width,Height));bitmap.Save(Path.Combine(TestDirectory,"gui-"+i+".png"));}
                }
                File.WriteAllText(Path.Combine(TestDirectory,"gui-result.txt"),"PASS: native GUI, authenticated snapshot, hidden token, redaction, all four sections rendered.\r\n"+meta.Text,Encoding.UTF8);
                Environment.ExitCode=0;
            }catch(Exception e){Directory.CreateDirectory(TestDirectory);File.WriteAllText(Path.Combine(TestDirectory,"gui-result.txt"),AgentClient.Redact(e.ToString()));Environment.ExitCode=1;}
            finally{busy=false;Close();}
        }
    }
}
