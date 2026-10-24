using System.IO;
using System.Net;
using System.Text.RegularExpressions;
using System.Windows;
using Microsoft.Win32;
using PPSetup.Ui;

namespace PPSetup.Server.Pages;

public partial class ServerPage : WizardPage
{
    static readonly Regex HostName = new(@"^(?=.{1,253}$)([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$");

    readonly SetupState state;

    public ServerPage(SetupState state)
    {
        this.state = state;
        InitializeComponent();
    }

    public override string Step => "Server";
    public override string Heading => "Your server";
    public override string Subheading => "How people will find it, and where it keeps its data.";
    public override bool Skip => state.Upgrade;

    public override void OnShown()
    {
        var p = state.Plan;
        if (NameBox.Text.Length == 0)
        {
            NameBox.Text = p.Name;
            HostsBox.Text = string.Join(", ", p.Hosts);
            PortBox.Text = p.Port.ToString();
            DataBox.Text = p.DataDir;
            ProgramBox.Text = p.InstallDir;
        }
        NameBox.Focus();
    }

    void BrowseData(object sender, RoutedEventArgs e) => Browse(DataBox, "Choose the data folder");

    void BrowseProgram(object sender, RoutedEventArgs e) => Browse(ProgramBox, "Choose the program folder");

    void Browse(System.Windows.Controls.TextBox box, string title)
    {
        var d = new OpenFolderDialog { Title = title, InitialDirectory = Directory.Exists(box.Text) ? box.Text : "" };
        if (d.ShowDialog(Window.GetWindow(this)) == true) box.Text = d.FolderName;
    }

    static List<string> SplitHosts(string s) =>
        s.Split([',', ';', ' '], StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries).Distinct().ToList();

    static bool FullPath(string p) => Path.IsPathFullyQualified(p) && Path.GetPathRoot(p)?.TrimEnd('\\') != p.TrimEnd('\\');

    string? Validate(out List<string> hosts, out int port)
    {
        hosts = SplitHosts(HostsBox.Text);
        port = 0;
        var name = NameBox.Text.Trim();
        if (name.Length is < 1 or > 80) return "Give the server a name (up to 80 characters).";
        if (hosts.Count is < 1 or > 20) return "Give between 1 and 20 names or IP addresses.";
        foreach (var h in hosts)
            if (!IPAddress.TryParse(h, out _) && !HostName.IsMatch(h))
                return $"\"{h}\" isn't a valid host name or IP address.";
        if (!int.TryParse(PortBox.Text, out port) || port is < 1 or > 65535 || port == 8444)
            return "The port must be a number from 1 to 65535 (8444 is reserved for the server's private admin port).";
        if (!FullPath(DataBox.Text)) return "The data folder must be a full path, and not a whole drive.";
        if (!FullPath(ProgramBox.Text)) return "The program folder must be a full path, and not a whole drive.";
        return null;
    }

    public override async Task<bool> OnNextAsync()
    {
        Problem.Visibility = Visibility.Collapsed;
        if (Validate(out var hosts, out var port) is { } problem)
        {
            Show(problem);
            return false;
        }
        var p = state.Plan;
        p.Name = NameBox.Text.Trim();
        p.Hosts = hosts;
        p.Port = port;
        p.DataDir = DataBox.Text.Trim();
        p.InstallDir = ProgramBox.Text.Trim();
        // Look again with the chosen folder and port: encryption and port checks depend on them.
        var probe = await state.Tool.ProbeAsync(p.DataDir, p.Port);
        state.Probe.Checks = probe.Checks;
        return true;
    }

    void Show(string message)
    {
        Problem.Text = message;
        Problem.Visibility = Visibility.Visible;
    }
}
