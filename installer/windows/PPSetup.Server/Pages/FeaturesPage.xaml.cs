using System.IO;
using System.Windows;
using Microsoft.Win32;
using PPSetup.Ui;

namespace PPSetup.Server.Pages;

public partial class FeaturesPage : WizardPage
{
    readonly SetupState state;
    bool loaded;

    public FeaturesPage(SetupState state)
    {
        this.state = state;
        InitializeComponent();
        Firewall.Checked += (_, _) => Sync();
        Firewall.Unchecked += (_, _) => Sync();
        Backups.Checked += (_, _) => Sync();
        Backups.Unchecked += (_, _) => Sync();
        foreach (var r in new[] { CertLocal, CertAcme, CertFiles }) r.Checked += (_, _) => Sync();
    }

    public override string Step => "Features";
    public override string Heading => "Choose what to set up";
    public override string Subheading => "Everything is on by default. Untick anything you don't want.";
    public override bool Skip => state.Upgrade;

    public override void OnShown()
    {
        var p = state.Plan;
        if (!loaded)
        {
            AutoStart.IsChecked = p.AutoStart;
            Firewall.IsChecked = p.Firewall;
            LanOnly.IsChecked = p.LanOnly;
            AllNetworks.IsChecked = !p.LanOnly;
            Backups.IsChecked = p.Backups;
            BackupDir.Text = p.BackupDir;
            BackupKeep.Text = p.BackupKeep.ToString();
            Tray.IsChecked = p.Tray;
            Shortcuts.IsChecked = p.Shortcuts;
            FirstInvite.IsChecked = p.FirstInvite;
            Calls.IsChecked = p.Calls;
            TrustCa.IsChecked = p.TrustCa;
            CertLocal.IsChecked = p.TlsMode is "" or "local-ca";
            CertAcme.IsChecked = p.TlsMode == "acme";
            CertFiles.IsChecked = p.TlsMode == "files";
            AcmeDomain.Text = p.AcmeDomain.Length > 0 ? p.AcmeDomain : p.Hosts.FirstOrDefault(h => h.Contains('.') && !char.IsDigit(h[^1])) ?? "";
            AcmeEmail.Text = p.AcmeEmail;
            CertFile.Text = p.CertFile;
            KeyFile.Text = p.KeyFile;
            loaded = true;
        }
        var enc = state.Probe.Find("encryption");
        var encrypted = enc?.State is null or "ok";
        UnencryptedPanel.Visibility = encrypted ? Visibility.Collapsed : Visibility.Visible;
        if (encrypted) Unencrypted.IsChecked = false;
        EncryptionNote.Text = "The data folder's drive isn't protected by BitLocker. ParallelPath only runs on an encrypted drive: " +
                              "turn on BitLocker (Settings › Privacy & security › Device encryption), or go Back and choose a folder on a BitLocker drive.";
        Sync();
    }

    void Sync()
    {
        FirewallScope.IsEnabled = Firewall.IsChecked == true;
        BackupOptions.IsEnabled = Backups.IsChecked == true;
        TrustCa.IsEnabled = CertLocal.IsChecked == true;
        AcmeOptions.IsEnabled = CertAcme.IsChecked == true;
        FileOptions.IsEnabled = CertFiles.IsChecked == true;
    }

    /// <summary>For automated tests in a sandbox (which has no BitLocker).</summary>
    public void AllowUnencryptedForTest() => Unencrypted.IsChecked = true;

    void BrowseBackups(object sender, RoutedEventArgs e)
    {
        var d = new OpenFolderDialog { Title = "Choose the backup folder", InitialDirectory = Directory.Exists(BackupDir.Text) ? BackupDir.Text : "" };
        if (d.ShowDialog(Window.GetWindow(this)) == true) BackupDir.Text = d.FolderName;
    }

    public override Task<bool> OnNextAsync()
    {
        Problem.Visibility = Visibility.Collapsed;
        var p = state.Plan;
        p.AutoStart = AutoStart.IsChecked == true;
        p.Firewall = Firewall.IsChecked == true;
        p.LanOnly = LanOnly.IsChecked == true;
        p.Backups = Backups.IsChecked == true;
        p.Tray = Tray.IsChecked == true;
        p.Shortcuts = Shortcuts.IsChecked == true;
        p.FirstInvite = FirstInvite.IsChecked == true;
        p.Unencrypted = UnencryptedPanel.Visibility == Visibility.Visible && Unencrypted.IsChecked == true;
        p.Calls = Calls.IsChecked == true;
        p.TlsMode = CertAcme.IsChecked == true ? "acme" : CertFiles.IsChecked == true ? "files" : "local-ca";
        p.TrustCa = p.TlsMode == "local-ca" && TrustCa.IsChecked == true;
        p.AcmeDomain = AcmeDomain.Text.Trim();
        p.AcmeEmail = AcmeEmail.Text.Trim();
        p.CertFile = CertFile.Text.Trim();
        p.KeyFile = KeyFile.Text.Trim();
        if (p.TlsMode == "acme" && (!p.AcmeDomain.Contains('.') || p.AcmeDomain.Contains(' ')))
            return Fail("Enter the server's public domain name for Let's Encrypt (for example family.example.org).");
        if (p.TlsMode == "files" && (!Path.IsPathFullyQualified(p.CertFile) || !Path.IsPathFullyQualified(p.KeyFile)))
            return Fail("Give the full paths of your certificate and private key files.");
        if (p.Backups)
        {
            if (!Path.IsPathFullyQualified(BackupDir.Text) || Path.GetPathRoot(BackupDir.Text)?.TrimEnd('\\') == BackupDir.Text.TrimEnd('\\'))
                return Fail("The backup folder must be a full path, and not a whole drive.");
            if (!int.TryParse(BackupKeep.Text, out var keep) || keep is < 1 or > 365)
                return Fail("Keep between 1 and 365 days of backups.");
            p.BackupDir = BackupDir.Text.Trim();
            p.BackupKeep = keep;
        }
        if (UnencryptedPanel.Visibility == Visibility.Visible && !p.Unencrypted)
            return Fail("The data folder needs a BitLocker-encrypted drive (or tick the testing-only option).");
        return Task.FromResult(true);
    }

    Task<bool> Fail(string message)
    {
        Problem.Text = message;
        Problem.Visibility = Visibility.Visible;
        return Task.FromResult(false);
    }
}
