using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace SmaManager {
    internal static class Maintenance {
        const string UpgradeCode="{48D2A5BA-96F0-4457-98A0-CBE35CE7247C}";
        [DllImport("msi.dll",CharSet=CharSet.Unicode)] static extern uint MsiOpenDatabase(string path,IntPtr persist,out uint database);
        [DllImport("msi.dll",CharSet=CharSet.Unicode)] static extern uint MsiDatabaseOpenView(uint database,string query,out uint view);
        [DllImport("msi.dll")] static extern uint MsiViewExecute(uint view,uint record);
        [DllImport("msi.dll")] static extern uint MsiViewFetch(uint view,out uint record);
        [DllImport("msi.dll",CharSet=CharSet.Unicode)] static extern uint MsiRecordGetString(uint record,uint field,StringBuilder value,ref uint size);
        [DllImport("msi.dll")] static extern uint MsiCloseHandle(uint handle);
        static string Property(uint db,string name){
            uint view=0,record=0;
            try{
                if(MsiDatabaseOpenView(db,"SELECT `Value` FROM `Property` WHERE `Property`='"+name+"'",out view)!=0||MsiViewExecute(view,0)!=0||MsiViewFetch(view,out record)!=0)throw new InvalidOperationException("无法读取 MSI 属性。");
                var text=new StringBuilder(1024);uint length=1024;
                if(MsiRecordGetString(record,1,text,ref length)!=0)throw new InvalidOperationException("MSI 属性无效。");
                return text.ToString();
            }finally{if(record!=0)MsiCloseHandle(record);if(view!=0)MsiCloseHandle(view);}
        }
        public static string VerifyMsi(string path){
            if(!File.Exists(path)||!Path.GetExtension(path).Equals(".msi",StringComparison.OrdinalIgnoreCase))throw new InvalidOperationException("请选择 SMA 的 MSI 安装包。");
            uint db=0;
            try{
                if(MsiOpenDatabase(path,IntPtr.Zero,out db)!=0)throw new InvalidOperationException("无法打开 MSI，请重新下载完整安装包。");
                if(!Property(db,"UpgradeCode").Equals(UpgradeCode,StringComparison.OrdinalIgnoreCase)||Property(db,"ProductName")!="SMA")throw new InvalidOperationException("所选文件不是 SMA 安装包。");
                return Property(db,"ProductVersion");
            }finally{if(db!=0)MsiCloseHandle(db);}
        }
        public static void ProtectDirectory(string path){
            var security=new DirectorySecurity();security.SetAccessRuleProtection(true,false);
            foreach(string sid in new[]{"S-1-5-18","S-1-5-32-544"})security.AddAccessRule(new FileSystemAccessRule(new SecurityIdentifier(sid),FileSystemRights.FullControl,InheritanceFlags.ContainerInherit|InheritanceFlags.ObjectInherit,PropagationFlags.None,AccessControlType.Allow));
            new DirectoryInfo(path).SetAccessControl(security);
        }
        public static void Launch(string action,string package){
            string directory=Path.Combine(Path.GetTempPath(),"sma-maintenance-"+Guid.NewGuid().ToString("N"));
            Directory.CreateDirectory(directory);ProtectDirectory(directory);
            string executable=Path.Combine(directory,"sma-gui.exe");
            File.Copy(Application.ExecutablePath,executable);
            File.Copy(Application.ExecutablePath+".config",executable+".config");
            File.Copy(Path.Combine(AgentClient.Root,"uninstall-sma.ps1"),Path.Combine(directory,"uninstall-sma.ps1"));
            var info=new ProcessStartInfo(executable,"--maintenance "+AgentClient.Quote(action)+" "+AgentClient.Quote(package)+" "+Process.GetCurrentProcess().Id){UseShellExecute=true,WorkingDirectory=directory};
            Process.Start(info);
        }
        public static Form Window(string action,string package,int parent){
            var form=new Form {Text=action=="upgrade"?"SMA 升级":"SMA 卸载",ClientSize=new Size(620,320),StartPosition=FormStartPosition.CenterScreen,Font=new Font("Microsoft YaHei UI",10),BackColor=Color.White,AutoScaleMode=AutoScaleMode.Dpi};
            var title=new Label{Text="正在处理，请稍候…",Dock=DockStyle.Top,Height=65,Padding=new Padding(24,20,24,0),Font=new Font(form.Font.FontFamily,17,FontStyle.Bold)};
            var details=new TextBox{Multiline=true,ReadOnly=true,Dock=DockStyle.Fill,BorderStyle=BorderStyle.None,BackColor=Color.White,ScrollBars=ScrollBars.Vertical};
            var body=new Panel{Dock=DockStyle.Fill,Padding=new Padding(24,8,24,8)};body.Controls.Add(details);
            var bar=new ProgressBar{Dock=DockStyle.Bottom,Height=5,Style=ProgressBarStyle.Marquee};
            var buttons=new FlowLayoutPanel{Dock=DockStyle.Bottom,Height=55,Padding=new Padding(20,8,0,0)};
            var close=new Button{Text="完成",Enabled=false,AutoSize=true,Height=35};close.Click+=delegate{form.Close();};buttons.Controls.Add(close);
            bool active=true;
            form.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(active)e.Cancel=true;};
            form.Controls.Add(body);form.Controls.Add(title);form.Controls.Add(bar);form.Controls.Add(buttons);
            form.Shown+=async delegate{
                string log=Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"maintenance.log");
                details.Text="正在等待管理界面退出，以便替换或移除程序。\r\n请勿关闭系统或中断 Windows Installer。\r\n完成后会在此处显示结果。\r\n日志："+log;
                try{
                    int code=await Task.Run(()=>{
                        try{using(var p=Process.GetProcessById(parent)){if(!p.WaitForExit(20000))throw new InvalidOperationException("原管理窗口未退出，请关闭后重试。");}}catch(ArgumentException){}
                        ProcessStartInfo info;
                        if(action=="upgrade"){
                            VerifyMsi(package);
                            info=new ProcessStartInfo(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System),"msiexec.exe"),"/i "+AgentClient.Quote(package)+" /qn /norestart /L*v "+AgentClient.Quote(log));
                        }else if(action=="uninstall"||action=="purge"){
                            info=new ProcessStartInfo(AgentClient.PowerShell,"-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "+AgentClient.Quote(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"uninstall-sma.ps1"))+" -Yes"+(action=="purge"?" -Purge":""));
                        }else throw new InvalidOperationException("未知维护操作。");
                        info.UseShellExecute=false;info.CreateNoWindow=true;info.WorkingDirectory=AppDomain.CurrentDomain.BaseDirectory;
                        using(var p=Process.Start(info)){p.WaitForExit();return p.ExitCode;}
                    });
                    if(code!=0&&code!=3010)throw new InvalidOperationException("安装器返回错误代码 "+code+"。请查看维护日志与系统安装日志。");
                    title.Text=action=="upgrade"?"升级完成":"卸载完成";
                    details.Text=(action=="upgrade"?"配置与 Token 已保留。请打开新版 SMA 管理中心检查状态。":action=="purge"?"程序与默认配置、Token、运行日志已清除。":"程序已卸载。配置、Token 和运行日志已保留，重装时可继续使用。")+"\r\n"+(code==3010?"Windows 要求重启，请在方便时手动重启。\r\n":"")+"维护目录："+AppDomain.CurrentDomain.BaseDirectory+"\r\n卸载的详细日志位于管理员临时目录 sma-uninstall-*.log。";
                    if(action=="upgrade"){
                        var open=new Button{Text="打开 SMA 管理中心",AutoSize=true,Height=35};
                        open.Click+=delegate{try{Process.Start(Path.Combine(AgentClient.Root,"sma-gui.exe"));form.Close();}catch(Exception e){MessageBox.Show(form,e.Message);}};buttons.Controls.Add(open);
                    }
                }catch(Exception e){title.Text="操作未完成";details.Text=AgentClient.Redact(e.Message)+"\r\n日志目录："+AppDomain.CurrentDomain.BaseDirectory;}
                finally{active=false;bar.Style=ProgressBarStyle.Blocks;bar.Value=100;close.Enabled=true;}
            };
            return form;
        }
    }
}
