using System;
using System.IO;
using System.Security.Principal;
using System.Windows.Forms;

[assembly:System.Reflection.AssemblyTitle("SMA 管理中心")]
[assembly:System.Reflection.AssemblyProduct("SMA")]
[assembly:System.Reflection.AssemblyVersion("0.3.1.0")]
[assembly:System.Reflection.AssemblyFileVersion("0.3.1.0")]

namespace SmaManager {
    internal static class Program {
        [STAThread] static void Main(string[] args){
            Environment.CurrentDirectory=Path.GetTempPath();
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            if(!new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator)){
                MessageBox.Show("请以管理员身份打开 SMA 管理中心，以便读取凭据和管理服务。","SMA",MessageBoxButtons.OK,MessageBoxIcon.Information);Environment.ExitCode=1;return;
            }
            try{
                if(args.Length==4&&args[0]=="--maintenance"){Application.Run(Maintenance.Window(args[1],args[2],int.Parse(args[3])));return;}
                var form=new ManagerForm();
                if(args.Length==2&&args[0]=="--self-test")form.TestDirectory=Path.GetFullPath(args[1]);
                Application.Run(form);
            }catch(Exception e){
                if(args.Length==2&&args[0]=="--self-test"){Directory.CreateDirectory(args[1]);File.WriteAllText(Path.Combine(args[1],"gui-fatal.txt"),AgentClient.Redact(e.ToString()));Environment.ExitCode=1;return;}
                MessageBox.Show(AgentClient.Redact(e.Message),"SMA 管理中心未能启动",MessageBoxButtons.OK,MessageBoxIcon.Error);Environment.ExitCode=1;
            }
        }
    }
}
