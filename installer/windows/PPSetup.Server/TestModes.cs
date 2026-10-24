using System.IO;
using System.Windows;
using PPSetup.Server.Pages;
using PPSetup.Ui;

namespace PPSetup.Server;

/// <summary>Clicks through a real installation like a person would, saving every page (used in Windows Sandbox).</summary>
static class AutoPilot
{
    public static void Attach(Application app, WizardWindow w, SetupState state, string dir)
    {
        var n = 0;
        w.PageShown += page => w.Dispatcher.InvokeAsync(async () =>
        {
            await Shots.Settle(400);
            switch (page)
            {
                case WelcomePage:
                    for (var i = 0; i < 300 && !page.CanGoNext; i++) await Task.Delay(100);
                    break;
                case LicencePage lp:
                    lp.Accept();
                    break;
                case FeaturesPage fp:
                    fp.AllowUnencryptedForTest(); // Windows Sandbox has no BitLocker
                    break;
                case InstallPage ip:
                    await Task.Delay(1500);
                    Shots.Save(w, Path.Combine(dir, $"{++n:00}-installing.png"));
                    return; // it moves on by itself, or stays on failure
                case FinishPage:
                    Shots.Save(w, Path.Combine(dir, $"{++n:00}-{page.Step.ToLowerInvariant()}.png"));
                    File.WriteAllText(Path.Combine(dir, "result.txt"), state.Result is { } r
                        ? $"ok {r.Version} {string.Join(",", r.Addresses ?? new())} invite={(string.IsNullOrEmpty(r.Invite) ? "no" : "yes")}"
                        : "no result");
                    w.Close();
                    return;
            }
            await Shots.Settle();
            Shots.Save(w, Path.Combine(dir, $"{++n:00}-{page.Step.ToLowerInvariant()}.png"));
            if (page is InstallPage) return;
            await w.PressNextAsync();
            if (w.Current == page && !page.CanGoNext)
            {
                Shots.Save(w, Path.Combine(dir, $"{++n:00}-stuck.png"));
                File.WriteAllText(Path.Combine(dir, "result.txt"), "stuck on " + page.Heading);
                w.Close();
            }
        });
        w.Closed += (_, _) =>
        {
            if (!File.Exists(Path.Combine(dir, "result.txt")))
                File.WriteAllText(Path.Combine(dir, "result.txt"), "closed early");
        };
    }
}

/// <summary>Renders every page with fictional data, for the documentation (nothing is installed).</summary>
static class Screenshots
{
    public static int Run(Application app, string dir)
    {
        var state = new SetupState { Tool = ServerTool.None() };
        var probe = new Probe
        {
            Os = "windows", Version = Program.Version, Admin = true, Hostname = "FAMILY-PC", Addresses = ["192.0.2.10"],
            Checks =
            [
                new Check { Id = "os", Title = "Windows", State = "ok", Detail = "Windows 10.0 build 26100" },
                new Check { Id = "admin", Title = "Administrator", State = "ok", Detail = "yes" },
                new Check { Id = "keys", Title = "Key protection", State = "ok", Detail = "Windows DPAPI, bound to the service's own account on this PC" },
                new Check { Id = "encryption", Title = "Data folder on an encrypted volume", State = "ok", Detail = "encrypted" },
                new Check { Id = "port", Title = "Port free", State = "ok", Detail = "port 8443 is free" },
                new Check { Id = "packages", Title = "Dependencies", State = "ok", Detail = "none - everything used is built into Windows" },
            ],
            Defaults = new Plan
            {
                InstallDir = @"C:\Program Files\ParallelPath Server", DataDir = @"C:\ProgramData\ParallelPath Server\data",
                Name = "Family server", Hosts = ["family.example.org", "192.0.2.10"], BackupDir = @"D:\Backups\ParallelPath",
            },
        };
        var pages = Program.Pages(state).ToList();
        var w = new WizardWindow("Server Setup", Program.Version, pages);
        var n = 0;
        ((WelcomePage)pages[0]).UseProbe(probe);
        ((LicencePage)pages[1]).Accept();
        w.Loaded += async (_, _) =>
        {
            foreach (var page in pages)
            {
                if (page is InstallPage) continue;
                if (page is FinishPage)
                {
                    state.Result = new Result
                    {
                        Name = "Family server", Version = Program.Version, Addresses = ["family.example.org:8443", "192.0.2.10:8443"],
                        CertSha256 = "9c98c70cebdbec10aeb1100eda7702dbb7753a68a198dc2bca608dc4b910c443", ServerKey = "51556016ee0dd1669e126c7d591d6da7",
                        AtRest = "encrypted volume", Invite = "S.Example-not-a-real-invite-code-0000000000000",
                        InviteExpires = DateTimeOffset.Now.AddDays(7).ToString("O"), Logs = @"C:\ProgramData\ParallelPath Server\data\logs\server.log",
                    };
                }
                w.GoTo(page);
                await Shots.Settle(350);
                Shots.Save(w, Path.Combine(dir, $"{++n:00}-{page.GetType().Name.Replace("Page", "").ToLowerInvariant()}.png"));
            }
            w.Close();
        };
        Shots.ShowOffscreen(w);
        return app.Run();
    }
}
