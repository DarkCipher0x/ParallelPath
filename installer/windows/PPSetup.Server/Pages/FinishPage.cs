using System.Text;
using System.Windows;
using System.Windows.Controls;
using PPSetup.Ui;

namespace PPSetup.Server.Pages;

/// <summary>The running server's details: where to connect, what the apps will show, and the first invite code.</summary>
public sealed class FinishPage : WizardPage
{
    readonly SetupState state;
    readonly StackPanel stack = new();

    public FinishPage(SetupState state)
    {
        this.state = state;
        Content = new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Content = stack };
    }

    public override string Step => "Done";
    public override string Heading => state.Upgrade && string.IsNullOrEmpty(state.Result?.Invite) ? "Upgrade complete" : "ParallelPath Server is running";
    public override string Subheading => state.Upgrade
        ? $"Version {state.Result?.Version} is running with all your data, settings and keys."
        : "Install the ParallelPath app on each device, then connect with these details.";
    public override string NextText => "Finish";

    /// <summary>Opens ParallelPath Server Manager (the icon by the clock) as the signed-in person.</summary>
    public override Task<bool> OnNextAsync()
    {
        if (state.Plan.Tray && Manager.ServerInfo.Load() is { } info) ManagerApp.LaunchAsUser(info.InstallDir);
        return Task.FromResult(true);
    }
    public override bool CanGoBack => false;
    public override bool CanCancel => false;
    public override bool AskBeforeClosing => false;

    public override void OnShown()
    {
        stack.Children.Clear();
        if (state.Result is not { } r) return;
        foreach (var note in r.Notes ?? new())
            stack.Children.Add(new Banner { Tone = Tone.Warn, Text = note, Margin = new Thickness(0, 0, 0, 12) });
        if (!string.IsNullOrEmpty(r.Invite))
        {
            var expires = DateTimeOffset.TryParse(r.InviteExpires, out var t) ? " · until " + t.LocalDateTime.ToString("d MMM yyyy HH:mm") : "";
            stack.Children.Add(new CopyField { Label = "FIRST INVITE CODE - works once" + expires, Value = r.Invite, ValueSize = 15 });
        }
        var addresses = r.Addresses ?? new();
        stack.Children.Add(new CopyField { Label = "SERVER ADDRESS" + (addresses.Count > 1 ? "ES (any of these)" : ""), Value = string.Join("\n", addresses),
            Copied = addresses.FirstOrDefault() });
        stack.Children.Add(new CopyField { Label = "CERTIFICATE FINGERPRINT - the app shows this when connecting; it must match", Value = Group(r.CertSha256) });
        stack.Children.Add(new CopyField { Label = "SERVER KEY", Value = Group(r.ServerKey) });
        var copyAll = new Button { Content = "Copy all details", HorizontalAlignment = HorizontalAlignment.Left, Margin = new Thickness(0, 4, 0, 14) };
        copyAll.SetResourceReference(StyleProperty, "Secondary");
        copyAll.Click += (_, _) =>
        {
            copyAll.Content = SafeClipboard.TrySetText(Summary(r)) ? "Copied" : "Clipboard busy - try again";
        };
        stack.Children.Add(copyAll);
        var more = new TextBlock
        {
            Text = "More invite codes: Start › ParallelPath Server › Create an invite code.\n" +
                   $"Encryption: {r.AtRest}.  Logs: {r.Logs}",
        };
        more.SetResourceReference(StyleProperty, "Small");
        stack.Children.Add(more);
    }

    static string Group(string hex)
    {
        var b = new StringBuilder();
        for (var i = 0; i < hex.Length; i++)
        {
            if (i > 0 && i % 4 == 0) b.Append(' ');
            b.Append(char.ToUpperInvariant(hex[i]));
        }
        return b.ToString();
    }

    static string Summary(Result r) =>
        $"ParallelPath Server: {r.Name} ({r.Version})\n" +
        $"Address: {string.Join(", ", r.Addresses ?? new())}\n" +
        $"Certificate fingerprint: {Group(r.CertSha256)}\n" +
        $"Server key: {Group(r.ServerKey)}\n" +
        (string.IsNullOrEmpty(r.Invite) ? "" : $"Invite code: {r.Invite}\n");
}
