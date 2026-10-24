using System.IO;
using System.Reflection;
using System.Windows;
using PPSetup.Ui;

namespace PPSetup.App;

/// <summary>
/// ParallelPath Setup: installs the Windows app for the current user (no administrator rights).
///
///   ParallelPath-Setup.exe                      the setup wizard
///   ParallelPath-Setup.exe --screenshots DIR    renders every page (documentation; nothing is installed)
///   ParallelPath-Setup.exe --autopilot DIR      clicks through a real install, saving each page (sandbox tests)
/// </summary>
static class Program
{
    static string Version =>
        typeof(Program).Assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion ?? "dev";

    [STAThread]
    static int Main(string[] args)
    {
        var app = new Application { ShutdownMode = ShutdownMode.OnMainWindowClose };
        app.Resources.MergedDictionaries.Add(new ResourceDictionary
        {
            Source = new Uri("pack://application:,,,/PPSetup.Ui;component/Themes/ParallelPath.xaml"),
        });
        Guard.Install(app, "ParallelPath Setup");
        var screenshots = args is ["--screenshots", _];
        var state = new AppState { Version = Version, Existing = screenshots ? null : AppInstaller.Existing() };
        var pages = new List<WizardPage>
        {
            new WelcomePage(state), new LicencePage { SkipIf = () => state.Upgrade }, new OptionsPage(state), new ChecksPage(state),
            new InstallPage(state), new FinishPage(state),
        };
        var w = new WizardWindow("Setup", Version, pages);
        if (screenshots) return Shoot(app, w, pages, args[1]);
        if (args is ["--autopilot", var dir]) AutoPilot(w, dir);
        return app.Run(w);
    }

    /// <summary>Renders every page except the install itself (documentation).</summary>
    static int Shoot(Application app, WizardWindow w, List<WizardPage> pages, string dir)
    {
        ((LicencePage)pages[1]).Accept();
        var n = 0;
        w.Loaded += async (_, _) =>
        {
            foreach (var page in pages.Where(p => p is not InstallPage))
            {
                w.GoTo(page);
                await Shots.Settle(350);
                Shots.Save(w, Path.Combine(dir, $"{++n:00}-{page.Step.ToLowerInvariant()}.png"));
            }
            w.Close();
        };
        Shots.ShowOffscreen(w);
        return app.Run();
    }

    /// <summary>Clicks through a real install like a person would, saving every page (Windows Sandbox tests).</summary>
    static void AutoPilot(WizardWindow w, string dir)
    {
        var n = 0;
        w.PageShown += page => w.Dispatcher.InvokeAsync(async () =>
        {
            await Shots.Settle(400);
            if (page is LicencePage lp) lp.Accept();
            if (page is FinishPage)
            {
                Shots.Save(w, Path.Combine(dir, $"{++n:00}-done.png"));
                File.WriteAllText(Path.Combine(dir, "result.txt"), "ok");
                await w.PressNextAsync(); // opens the app, as a person would
                return;
            }
            if (page is InstallPage)
            {
                await Task.Delay(800);
                Shots.Save(w, Path.Combine(dir, $"{++n:00}-installing.png"));
                return;
            }
            await Shots.Settle();
            Shots.Save(w, Path.Combine(dir, $"{++n:00}-{page.Step.ToLowerInvariant()}.png"));
            await w.PressNextAsync();
        });
    }
}
