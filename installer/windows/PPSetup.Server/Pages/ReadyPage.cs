using System.Windows;
using System.Windows.Controls;
using PPSetup.Ui;

namespace PPSetup.Server.Pages;

/// <summary>A summary of what will happen, before anything is changed.</summary>
public sealed class ReadyPage : WizardPage
{
    readonly SetupState state;
    readonly Grid grid = new();
    readonly TextBlock intro = new();

    public ReadyPage(SetupState state)
    {
        this.state = state;
        grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(170) });
        grid.ColumnDefinitions.Add(new ColumnDefinition());
        var card = new Border { Child = grid };
        var stack = new StackPanel();
        stack.Children.Add(intro);
        stack.Children.Add(card);
        Content = new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Content = stack };
        Loaded += (_, _) =>
        {
            card.Style = (Style)FindResource("Card");
            intro.Style = (Style)FindResource("Body");
            intro.Margin = new Thickness(0, 0, 0, 14);
        };
    }

    public override string Step => "Install";
    public override string Heading => state.Upgrade ? "Ready to upgrade" : "Ready to install";
    public override string Subheading => state.Upgrade ? "" : "Check the details, then install. It takes about half a minute.";
    public override string NextText => state.Upgrade ? "Upgrade" : "Install";

    public override void OnShown()
    {
        grid.Children.Clear();
        grid.RowDefinitions.Clear();
        var p = state.Plan;
        if (state.Upgrade)
        {
            var e = state.Probe.Existing!;
            intro.Text = $"ParallelPath Server {e.Version} will be upgraded to {state.Probe.Version}. All data, settings and keys are kept; " +
                         "the server stops for a few seconds while the program is replaced.";
            Row("Data folder", e.DataDir);
            Row("Program folder", string.IsNullOrEmpty(e.InstallDir) ? p.InstallDir : e.InstallDir);
            return;
        }
        intro.Text = "Setup will install the server as a Windows service, lock down its data folder and start it.";
        Row("Name", p.Name);
        Row("Reached at", string.Join(", ", p.Hosts) + "  ·  port " + p.Port);
        Row("Data folder", p.DataDir);
        Row("Program folder", p.InstallDir);
        Row("Starts", p.AutoStart ? "Automatically with Windows" : "By hand (Services)");
        Row("Firewall", p.Firewall ? (p.LanOnly ? "Allowed on private and domain networks" : "Allowed on all networks") : "No rule");
        Row("Backups", p.Backups ? $"Daily to {p.BackupDir}, kept {p.BackupKeep} days" : "Off");
        Row("Icon by the clock", p.Tray ? "ParallelPath Server Manager, at sign-in" : "No");
        Row("Shortcuts", p.Shortcuts ? "Start › ParallelPath Server" : "None");
        Row("Invite code", p.FirstInvite ? "Created at the end" : "Not now");
        if (p.Unencrypted) Row("Encryption", "Drive NOT encrypted - testing only", "Warm");
    }

    void Row(string label, string value, string brush = "Fg")
    {
        var r = grid.RowDefinitions.Count;
        grid.RowDefinitions.Add(new RowDefinition());
        var l = new TextBlock { Text = label, Margin = new Thickness(0, 5, 12, 5), FontSize = 13.5 };
        var v = new TextBlock { Text = value, Margin = new Thickness(0, 5, 0, 5), TextWrapping = TextWrapping.Wrap, FontSize = 13.5 };
        l.SetResourceReference(TextBlock.ForegroundProperty, "FgMute");
        v.SetResourceReference(TextBlock.ForegroundProperty, brush);
        Grid.SetRow(l, r);
        Grid.SetRow(v, r);
        Grid.SetColumn(v, 1);
        grid.Children.Add(l);
        grid.Children.Add(v);
    }
}
