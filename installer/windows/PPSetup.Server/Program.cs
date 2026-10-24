using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Reflection;
using System.Security.Principal;
using System.Windows;
using PPSetup.Server.Manager;
using PPSetup.Server.Pages;
using PPSetup.Ui;

namespace PPSetup.Server;

/// <summary>
/// ParallelPath Server Setup for Windows, which is also ParallelPath Server Manager.
///
///   ParallelPath-Server-Setup.exe                      the setup wizard (asks for administrator rights)
///   ParallelPath Server Manager.exe [--open | --tray]   the icon by the clock (--tray: without opening the window)
///   ... --setup                                         the installed Manager runs setup again (repair)
///   ... --screenshots DIR                               renders every setup page with fictional data (documentation)
///   ... --autopilot DIR                                 clicks through a real install, saving each page (sandbox tests)
/// </summary>
static class Program
{
    public const string ManagerFileName = "ParallelPath Server Manager.exe";

    public static string Version =>
        typeof(Program).Assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion ?? "dev";

    [STAThread]
    static int Main(string[] args)
    {
        var app = new Application { ShutdownMode = ShutdownMode.OnMainWindowClose };
        app.Resources.MergedDictionaries.Add(new ResourceDictionary
        {
            Source = new Uri("pack://application:,,,/PPSetup.Ui;component/Themes/ParallelPath.xaml"),
        });
        var isManager = string.Equals(Path.GetFileName(Environment.ProcessPath), ManagerFileName, StringComparison.OrdinalIgnoreCase);
        Guard.Install(app, isManager ? "ParallelPath Server Manager" : "ParallelPath Server Setup");
        switch (args)
        {
            case ["--screenshots", var shotsDir]:
                return Screenshots.Run(app, shotsDir);
            case ["--tray"]:
                return ManagerApp.Run(app, open: false);
            case ["--snapshot-admin", var adminShot]:
                return ManagerApp.Snapshot(app, adminShot, admin: true);
            case ["--snapshot", var snapshot]:
                return ManagerApp.Snapshot(app, snapshot, admin: false);
            case ["--open"]:
            case [] when isManager:
                return ManagerApp.Run(app, open: true);
        }
        if (!Elevated()) return RelaunchElevated(args.Where(a => a != "--setup"));

        ServerTool tool;
        try
        {
            tool = ServerTool.Unpack();
        }
        catch (Exception ex)
        {
            MessageBox.Show(ex.Message, "ParallelPath Server Setup", MessageBoxButton.OK, MessageBoxImage.Error);
            return 1;
        }
        using (tool)
        {
            var state = new SetupState { Tool = tool };
            var w = new WizardWindow("Server Setup", Version, Pages(state));
            if (args is ["--autopilot", var outDir]) AutoPilot.Attach(app, w, state, outDir);
            return app.Run(w);
        }
    }

    public static IEnumerable<WizardPage> Pages(SetupState s) =>
    [
        new WelcomePage(s), new LicencePage { SkipIf = () => s.Upgrade }, new ServerPage(s), new FeaturesPage(s),
        new ChecksPage(s), new ReadyPage(s), new InstallPage(s), new FinishPage(s),
    ];

    static bool Elevated() => new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator);

    /// <summary>Setup changes the system, so it runs as administrator: one UAC prompt, then this copy steps aside.</summary>
    static int RelaunchElevated(IEnumerable<string> args)
    {
        var psi = new ProcessStartInfo(Environment.ProcessPath!) { UseShellExecute = true, Verb = "runas" };
        foreach (var a in args) psi.ArgumentList.Add(a);
        try
        {
            Process.Start(psi)?.Dispose();
            return 0;
        }
        catch (Win32Exception ex) when (ex.NativeErrorCode == 1223) // the person said no
        {
            return 1;
        }
    }
}

/// <summary>The Manager: one icon per person, however many times it's started (a second start opens the window).</summary>
static class ManagerApp
{
    const string Name = @"Local\DarkCipherSystems.ParallelPathServer.Manager";

    public static int Run(Application app, bool open)
    {
        using var single = new Mutex(true, Name, out var first);
        using var signal = new EventWaitHandle(false, EventResetMode.AutoReset, Name + ".Open");
        if (!first)
        {
            if (open) signal.Set();
            return 0;
        }
        app.ShutdownMode = ShutdownMode.OnExplicitShutdown;
        using var tray = new TrayIcon(app.Shutdown);
        new Thread(() =>
        {
            while (signal.WaitOne()) app.Dispatcher.BeginInvoke(tray.Show);
        }) { IsBackground = true }.Start();
        if (open) tray.Show();
        return app.Run();
    }

    /// <summary>Test mode: opens the window, waits for the server's page, saves a picture of it and a status line, exits.</summary>
    public static int Snapshot(Application app, string file, bool admin)
    {
        app.ShutdownMode = ShutdownMode.OnExplicitShutdown;
        var tray = new TrayIcon(app.Shutdown);
        tray.Show();
        app.Dispatcher.InvokeAsync(async () =>
        {
            if (admin)
            {
                for (var i = 0; i < 120 && !tray.Window.PageLoaded; i++) await Task.Delay(250);
                await tray.Window.OpenAdmin();
                await Task.Delay(500);
            }
            for (var i = 0; i < 120 && !tray.Window.PageLoaded; i++) await Task.Delay(250);
            await Shots.Settle(admin ? 3000 : 800);
            Shots.Save(tray.Window, file);
            // The page itself is drawn by Edge in its own native window, which a WPF picture can't include.
            if (tray.Window.PageLoaded) await tray.Window.CapturePage(Path.ChangeExtension(file, ".page.png"));
            if (admin && tray.Window.PageLoaded)
                foreach (var page in new[] { "invites", "accounts", "families", "backups", "settings", "logs", "audit", "admins" })
                {
                    await tray.Window.RunScript($"location.hash='{page}'");
                    await Task.Delay(1500);
                    await tray.Window.CapturePage(Path.ChangeExtension(file, "." + page + ".png"));
                }
            File.WriteAllText(file + ".txt", $"loaded={tray.Window.PageLoaded} state={ServerInfo.State()} problem={tray.Window.Problem}\n");
            tray.Dispose();
            app.Shutdown();
        });
        return app.Run();
    }

    /// <summary>
    /// Starts the installed Manager as the signed-in person. Setup runs elevated, and anything it started directly
    /// would be elevated too; asking Explorer to open it gives it the person's ordinary rights.
    /// </summary>
    public static void LaunchAsUser(string installDir)
    {
        var exe = Path.Combine(installDir, Program.ManagerFileName);
        if (File.Exists(exe))
            Process.Start(new ProcessStartInfo(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Windows), "explorer.exe"))
                { ArgumentList = { exe }, UseShellExecute = false })?.Dispose();
    }
}
