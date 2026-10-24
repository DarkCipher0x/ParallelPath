using System.IO;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using Microsoft.Win32;
using PPSetup.Ui;

namespace PPSetup.App;

/// <summary>Everything the pages share.</summary>
public sealed class AppState
{
    public string Version { get; init; } = "";
    public (string Version, string Dir)? Existing { get; init; }
    public string Dir { get; set; } = AppInstaller.DefaultDir;
    public bool Desktop { get; set; } = true;
    public bool StartMenu { get; set; } = true;
    public bool Launch { get; set; } = true;
    public bool Upgrade => Existing != null;
}

/// <summary>Helpers the code-built pages share.</summary>
static class Ui
{
    public static TextBlock Text(string text, string style, double top = 0) =>
        new() { Text = text, Margin = new Thickness(0, top, 0, 0), Style = (Style)Application.Current.FindResource(style) };

    public static UIElement Scroll(UIElement content) =>
        new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Content = content };

    public static CheckBox Feature(string title, string detail, bool on)
    {
        var stack = new StackPanel();
        stack.Children.Add(Text(title, "H2"));
        stack.Children.Add(Text(detail, "Small"));
        return new CheckBox { Content = stack, IsChecked = on, Style = (Style)Application.Current.FindResource("FeatureCard") };
    }
}

public sealed class WelcomePage : WizardPage
{
    readonly AppState s;

    public WelcomePage(AppState s)
    {
        this.s = s;
        var stack = new StackPanel();
        if (s.Existing is { } e)
        {
            var same = e.Version == s.Version;
            stack.Children.Add(new Banner
            {
                Tone = Tone.Info, Margin = new Thickness(0, 0, 0, 18),
                Text = $"ParallelPath {e.Version} is installed. Setup will {(same ? "repair it" : $"update it to {s.Version}")}. " +
                       "Your encrypted data, keys and settings stay exactly as they are.",
            });
        }
        stack.Children.Add(Ui.Text("ParallelPath keeps co-parenting calm and organised: messages, photos and videos, the family's " +
                                   "shared record, and an audit log nobody can quietly change - all end-to-end encrypted with " +
                                   "post-quantum cryptography.", "Body"));
        var grid = new UniformGrid { Columns = 2, Margin = new Thickness(-6, 18, -6, 0) };
        foreach (var (t, d) in new[]
                 {
                     ("Private by design", "Your keys never leave this PC, locked by your passphrase (Argon2id)."),
                     ("Nothing can be quietly changed", "Edits need a reason, history always stays, every action is in the audit log."),
                     ("Everything included", "Its own runtime and post-quantum engine - nothing else to install."),
                     ("Just for you", "Installs for your Windows account only. No administrator rights needed."),
                 })
        {
            var card = new Border { Margin = new Thickness(6), Style = (Style)Application.Current.FindResource("Card") };
            var inner = new StackPanel();
            inner.Children.Add(Ui.Text(t, "H2"));
            inner.Children.Add(Ui.Text(d, "Small", 4));
            card.Child = inner;
            grid.Children.Add(card);
        }
        stack.Children.Add(grid);
        Content = Ui.Scroll(stack);
    }

    public override string Step => "Welcome";
    public override string Heading => s.Upgrade ? "Update ParallelPath" : "Welcome to ParallelPath";
    public override string Subheading => "Post-quantum secure co-parenting for Windows.";
    public override bool CanGoBack => false;
}

public sealed class OptionsPage : WizardPage
{
    readonly AppState s;
    readonly TextBox dir = new();
    readonly CheckBox desktop, startMenu;
    readonly Banner problem = new() { Tone = Tone.Error, Visibility = Visibility.Collapsed, Margin = new Thickness(0, 0, 0, 14) };

    public OptionsPage(AppState s)
    {
        this.s = s;
        var stack = new StackPanel();
        stack.Children.Add(problem);
        stack.Children.Add(Ui.Text("INSTALL FOLDER", "Label"));
        var browse = new Button { Content = "Browse…", Margin = new Thickness(10, 0, 0, 0), Style = (Style)Application.Current.FindResource("Secondary") };
        browse.Click += (_, _) =>
        {
            var d = new OpenFolderDialog { Title = "Choose where to install ParallelPath" };
            if (d.ShowDialog(Window.GetWindow(this)) == true) dir.Text = Path.Combine(d.FolderName, "ParallelPath");
        };
        var row = new DockPanel();
        DockPanel.SetDock(browse, Dock.Right);
        row.Children.Add(browse);
        row.Children.Add(dir);
        stack.Children.Add(row);
        stack.Children.Add(Ui.Text("Your encrypted data is kept separately, in your own AppData folder.", "Small", 5));
        desktop = Ui.Feature("Desktop shortcut", "A ParallelPath icon on your desktop.", s.Desktop);
        startMenu = Ui.Feature("Start-menu shortcut", "ParallelPath in Start, so you can search for it.", s.StartMenu);
        desktop.Margin = new Thickness(0, 22, 0, 10);
        stack.Children.Add(desktop);
        stack.Children.Add(startMenu);
        Content = Ui.Scroll(stack);
    }

    public override string Step => "Options";
    public override string Heading => "Choose how to install";
    public override string Subheading => "These defaults suit almost everyone.";
    public override bool Skip => s.Upgrade;

    public override void OnShown()
    {
        if (dir.Text.Length == 0) dir.Text = s.Dir;
    }

    public override Task<bool> OnNextAsync()
    {
        var path = dir.Text.Trim();
        if (!Path.IsPathFullyQualified(path) || Path.GetPathRoot(path)?.TrimEnd('\\') == path.TrimEnd('\\'))
        {
            problem.Text = "The install folder must be a full path, and not a whole drive.";
            problem.Visibility = Visibility.Visible;
            return Task.FromResult(false);
        }
        if (Directory.Exists(path) && Directory.EnumerateFileSystemEntries(path).Any() && !File.Exists(Path.Combine(path, "ParallelPath.exe")))
        {
            problem.Text = "That folder already holds other files. Choose a new or empty folder.";
            problem.Visibility = Visibility.Visible;
            return Task.FromResult(false);
        }
        s.Dir = path;
        s.Desktop = desktop.IsChecked == true;
        s.StartMenu = startMenu.IsChecked == true;
        return Task.FromResult(true);
    }
}

public sealed class ChecksPage : WizardPage
{
    readonly AppState s;
    readonly ProgressList list = new();
    readonly Banner summary = new() { Margin = new Thickness(0, 6, 0, 0) };
    readonly Button retry;
    bool blocked;

    public ChecksPage(AppState s)
    {
        this.s = s;
        retry = new Button { Content = "Check again", HorizontalAlignment = HorizontalAlignment.Left, Margin = new Thickness(0, 12, 0, 0),
            Style = (Style)Application.Current.FindResource("Secondary"), Visibility = Visibility.Collapsed };
        retry.Click += (_, _) => OnShown();
        var stack = new StackPanel();
        stack.Children.Add(list);
        stack.Children.Add(summary);
        stack.Children.Add(retry);
        Content = Ui.Scroll(stack);
    }

    public override string Step => "Checks";
    public override string Heading => "Checking this PC";
    public override string Subheading => "ParallelPath brings everything it needs - there are no other downloads.";
    public override bool CanGoNext => !blocked;

    public override void OnShown()
    {
        list.Rows.Clear();
        var v = Environment.OSVersion.Version;
        Add("Windows", $"Windows {v.Major}.{v.Minor} build {v.Build}", v.Major >= 10 && v.Build >= 17763, "Windows 10 1809 or newer is needed.");
        Add("64-bit", Environment.Is64BitOperatingSystem ? "yes" : "no", Environment.Is64BitOperatingSystem, "ParallelPath needs 64-bit Windows.");
        var free = AppInstaller.FreeBytes(s.Dir);
        Add("Disk space", $"{free / (1L << 30)} GB free (about 0.5 GB needed)", free > 600L << 20, "Free up some space on that drive.");
        if (s.Upgrade)
        {
            var running = AppInstaller.Running(s.Existing!.Value.Dir);
            Add("ParallelPath closed", running ? "it's still open" : "yes", !running, "Close ParallelPath (your work is saved), then press Check again.");
        }
        list.Rows.Add(new ProgressRow { Title = "Dependencies", Detail = "none - its own runtime and post-quantum engine are included", State = RowState.Done });
        blocked = list.Rows.Any(r => r.State == RowState.Failed);
        summary.Tone = blocked ? Tone.Error : Tone.Ok;
        summary.Text = blocked ? "Fix the problem above to continue." : "This PC is ready.";
        retry.Visibility = blocked ? Visibility.Visible : Visibility.Collapsed;
        Changed();
    }

    void Add(string title, string detail, bool ok, string fix) =>
        list.Rows.Add(new ProgressRow { Title = title, Detail = ok ? detail : detail + " - " + fix, State = ok ? RowState.Done : RowState.Failed });
}

public sealed class InstallPage : WizardPage
{
    readonly AppState s;
    readonly ProgressList list = new();
    readonly Banner failure = new() { Tone = Tone.Error, Visibility = Visibility.Collapsed, Margin = new Thickness(0, 0, 0, 14) };
    bool running, failed;

    public InstallPage(AppState s)
    {
        this.s = s;
        var stack = new StackPanel();
        stack.Children.Add(failure);
        stack.Children.Add(list);
        Content = Ui.Scroll(stack);
    }

    public override string Step => "Install";
    public override string Heading => failed ? "Setup didn't finish" : s.Upgrade ? "Updating…" : "Installing…";
    public override string Subheading => failed ? "Your existing installation and data are untouched." : "";
    public override bool ShowNext => failed;
    public override string NextText => "Close";
    public override bool CanGoBack => failed;
    public override bool CanCancel => false;
    public override bool CanClose => !running;
    public override bool AskBeforeClosing => false;

    public override async void OnShown()
    {
        if (running) return;
        running = true;
        failed = false;
        failure.Visibility = Visibility.Collapsed;
        list.Rows.Clear();
        Changed();
        var dir = s.Existing?.Dir ?? s.Dir;
        try
        {
            await RunStep("Installing ParallelPath", () => AppInstaller.InstallFiles(dir));
            await RunStep("Shortcuts", () => AppInstaller.Shortcuts(dir, s.Desktop, s.StartMenu), skip: s.Upgrade);
            await RunStep("Registering with Windows", () => AppInstaller.Register(dir, s.Version));
            running = false;
            Wizard.Forward();
        }
        catch (Exception ex)
        {
            running = false;
            failed = true;
            failure.Text = ex.Message;
            failure.Visibility = Visibility.Visible;
            Changed();
        }
    }

    async Task RunStep(string title, Func<string> work, bool skip = false)
    {
        var row = new ProgressRow { Title = title, State = skip ? RowState.Skipped : RowState.Running, Detail = skip ? "kept as they were" : "" };
        list.Rows.Add(row);
        if (skip) return;
        try
        {
            row.Detail = await Task.Run(work);
            row.State = RowState.Done;
        }
        catch (Exception ex)
        {
            row.State = RowState.Failed;
            row.Detail = ex.Message;
            throw;
        }
    }

    public override Task<bool> OnNextAsync()
    {
        Wizard.Close();
        return Task.FromResult(false);
    }
}

public sealed class FinishPage : WizardPage
{
    readonly AppState s;
    readonly CheckBox launch;

    public FinishPage(AppState s)
    {
        this.s = s;
        var stack = new StackPanel();
        stack.Children.Add(Ui.Text(s.Upgrade
            ? "ParallelPath is up to date. Open it and unlock with your passphrase as usual."
            : "To get started you'll need your family's server address and an invite code - from whoever runs your ParallelPath " +
              "server, or from the other parent if your family is already set up.", "Body"));
        launch = Ui.Feature("Open ParallelPath now", "", true);
        launch.Margin = new Thickness(0, 20, 0, 0);
        stack.Children.Add(launch);
        stack.Children.Add(Ui.Text("To remove it later: Settings › Apps › ParallelPath.", "Small", 14));
        Content = Ui.Scroll(stack);
    }

    public override string Step => "Done";
    public override string Heading => s.Upgrade ? "ParallelPath is updated" : "ParallelPath is installed";
    public override string NextText => "Finish";
    public override bool CanGoBack => false;
    public override bool CanCancel => false;
    public override bool AskBeforeClosing => false;

    public override Task<bool> OnNextAsync()
    {
        if (launch.IsChecked == true) AppInstaller.Launch(s.Existing?.Dir ?? s.Dir);
        return Task.FromResult(true);
    }
}
