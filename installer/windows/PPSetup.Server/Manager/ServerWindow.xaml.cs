using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using System.ServiceProcess;
using System.Windows;
using System.Windows.Media;
using Microsoft.Web.WebView2.Core;
using PPSetup.Ui;

namespace PPSetup.Server.Manager;

/// <summary>
/// The server's own page, shown in Microsoft's Edge engine (WebView2). The server's certificate is its own
/// (self-signed), so instead of warning, the window accepts exactly the certificate setup recorded - its SHA-256 is
/// pinned - and refuses any other. Nothing is added to Windows' trusted certificates.
/// </summary>
public partial class ServerWindow : Window
{
    ServerInfo? info;
    ServiceControllerStatus? state;
    bool webStarted;
    string? webProblem;
    Action? messageAction;

    public ServerWindow()
    {
        InitializeComponent();
        SourceInitialized += (_, _) => Native.DarkTitleBar(this);
    }

    /// <summary>Whether the server's page has loaded, and the last problem shown (for the --snapshot test mode).</summary>
    public bool PageLoaded { get; private set; }
    public string? Problem { get; private set; }

    /// <summary>Set when the Manager is exiting (otherwise closing the window only hides it).</summary>
    public bool Exiting { get; set; }

    protected override void OnClosing(CancelEventArgs e)
    {
        base.OnClosing(e);
        if (Exiting) return;
        e.Cancel = true;
        Hide();
    }

    Brush B(string key) => (Brush)FindResource(key);

    /// <summary>Shows the server's current state.</summary>
    public void Update(ServerInfo? newInfo, ServiceControllerStatus? newState)
    {
        var restarted = state != ServiceControllerStatus.Running && newState == ServiceControllerStatus.Running;
        info = newInfo;
        state = newState;
        Details.Text = info == null ? "Not installed on this PC" : $"{info.Name}  ·  {info.Version}  ·  {info.FirstAddress}";
        var (text, tone) = state switch
        {
            ServiceControllerStatus.Running => ("Running", "Ok"),
            ServiceControllerStatus.StartPending => ("Starting", "Warm"),
            ServiceControllerStatus.StopPending => ("Stopping", "Warm"),
            ServiceControllerStatus.Stopped => ("Stopped", "Danger"),
            null => ("Not installed", "FgMute"),
            _ => (state.ToString()!, "Warm"),
        };
        PillText.Text = text;
        PillText.Foreground = PillDot.Fill = Pill.BorderBrush = B(tone);
        var running = state == ServiceControllerStatus.Running;
        var stopped = state == ServiceControllerStatus.Stopped;
        PowerButton.Content = running ? "Restart" : "Start server";
        PowerButton.IsEnabled = info != null && (running || stopped);
        PowerButton.Style = (Style)FindResource(running ? "Secondary" : "Primary");
        AdminButton.IsEnabled = InviteButton.IsEnabled = BrowserButton.IsEnabled = running;
        CopyButton.IsEnabled = info != null;

        if (info == null)
            ShowMessage("ParallelPath Server isn't installed", "Run ParallelPath Server Setup to install it.", null, null);
        else if (stopped)
            ShowMessage("The server isn't running", "Start it so your family's devices can connect.", "Start server", Power);
        else if (!running)
            ShowMessage(text + "…", "This takes a few seconds.", null, null);
        else
            ShowWeb(restarted);
    }

    void ShowMessage(string title, string text, string? action, Action? onAction)
    {
        Problem = title + ": " + text;
        Web.Visibility = Visibility.Collapsed;
        Message.Visibility = Visibility.Visible;
        MessageTitle.Text = title;
        MessageText.Text = text;
        MessageAction.Content = action;
        MessageAction.Visibility = action == null ? Visibility.Collapsed : Visibility.Visible;
        messageAction = onAction;
    }

    async void ShowWeb(bool reload)
    {
        if (webProblem != null)
        {
            ShowMessage("The page can't be shown here", webProblem, "Open in browser", Browser);
            return;
        }
        Message.Visibility = Visibility.Collapsed;
        Web.Visibility = Visibility.Visible;
        if (webStarted)
        {
            if (reload) Web.Reload();
            return;
        }
        webStarted = true;
        try
        {
            var data = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "DarkCipher Systems", "ParallelPath Server Manager", "WebView2");
            var env = await CoreWebView2Environment.CreateAsync(null, data);
            await Web.EnsureCoreWebView2Async(env);
            var core = Web.CoreWebView2;
            core.Profile.PreferredColorScheme = CoreWebView2PreferredColorScheme.Dark; // matches this window
            core.Settings.AreDevToolsEnabled = false;
            core.Settings.AreDefaultContextMenusEnabled = false;
            core.Settings.IsStatusBarEnabled = false;
            core.Settings.IsPasswordAutosaveEnabled = false;
            core.Settings.IsGeneralAutofillEnabled = false;
            core.ServerCertificateErrorDetected += OnCertificate;
            core.NavigationStarting += OnNavigating;
            core.NewWindowRequested += (_, e) =>
            {
                e.Handled = true;
                OpenOutside(e.Uri);
            };
            core.NavigationCompleted += (_, e) =>
            {
                PageLoaded = e.IsSuccess;
                if (!e.IsSuccess && e.WebErrorStatus != CoreWebView2WebErrorStatus.OperationCanceled)
                    ShowMessage("The page didn't open", PageProblem(e.WebErrorStatus), "Try again", () => { ShowWeb(true); });
            };
            Web.Source = info!.LocalUrl;
        }
        catch (WebView2RuntimeNotFoundException)
        {
            webProblem = "This PC doesn't have the Microsoft Edge WebView2 Runtime, which shows the page inside this window. " +
                         "Install it from Microsoft (it comes with Windows 11), or open the page in your browser.";
            ShowWeb(false);
        }
    }

    string PageProblem(CoreWebView2WebErrorStatus status) => status switch
    {
        CoreWebView2WebErrorStatus.CertificateCommonNameIsIncorrect or CoreWebView2WebErrorStatus.CertificateExpired or
            CoreWebView2WebErrorStatus.ClientCertificateContainsErrors or CoreWebView2WebErrorStatus.CertificateRevoked or
            CoreWebView2WebErrorStatus.CertificateIsInvalid =>
            "The server's certificate doesn't match the one setup recorded, so the page was refused. If you didn't change the " +
            "server, something may be pretending to be it: run ParallelPath Server Setup again to check.",
        _ => $"It couldn't be reached ({status}). The server may still be starting.",
    };

    // The pin: only the certificate setup recorded (SHA-256 of the whole certificate), only on this PC's loopback.
    void OnCertificate(object? sender, CoreWebView2ServerCertificateErrorDetectedEventArgs e)
    {
        e.Action = CoreWebView2ServerCertificateErrorAction.Cancel;
        if (info == null || info.CertSha256.Length != 64) return;
        var uri = new Uri(e.RequestUri);
        if (uri.Host != "127.0.0.1" || uri.Port != info.Port) return;
        using var cert = e.ServerCertificate.ToX509Certificate2();
        if (Pinned(cert.RawData))
        {
            e.Action = CoreWebView2ServerCertificateErrorAction.AlwaysAllow; // the self-signed certificate itself
            return;
        }
        // The server's own certificate authority (tls_mode local-ca) is what's pinned: accept a certificate it issued.
        foreach (var pem in e.ServerCertificate.PemEncodedIssuerCertificateChain)
        {
            using var ca = X509Certificate2.CreateFromPem(pem);
            if (!Pinned(ca.RawData)) continue;
            using var chain = new X509Chain();
            chain.ChainPolicy.TrustMode = X509ChainTrustMode.CustomRootTrust;
            chain.ChainPolicy.CustomTrustStore.Add(ca);
            chain.ChainPolicy.RevocationMode = X509RevocationMode.NoCheck;
            if (chain.Build(cert)) e.Action = CoreWebView2ServerCertificateErrorAction.AlwaysAllow;
            return;
        }
    }

    bool Pinned(byte[] der)
    {
        var hash = Convert.ToHexString(SHA256.HashData(der)).ToLowerInvariant();
        return CryptographicOperations.FixedTimeEquals(System.Text.Encoding.ASCII.GetBytes(hash), System.Text.Encoding.ASCII.GetBytes(info!.CertSha256));
    }

    // The window only ever shows the server; links elsewhere open in the browser.
    void OnNavigating(object? sender, CoreWebView2NavigationStartingEventArgs e)
    {
        var uri = new Uri(e.Uri);
        if (info != null && uri.Scheme == "https" && uri.Host == "127.0.0.1" && uri.Port == info.Port) return;
        e.Cancel = true;
        OpenOutside(e.Uri);
    }

    static void OpenOutside(string url)
    {
        if (Uri.TryCreate(url, UriKind.Absolute, out var u) && (u.Scheme == "https" || u.Scheme == "http"))
            Process.Start(new ProcessStartInfo(u.ToString()) { UseShellExecute = true })?.Dispose();
    }

    /// <summary>Runs a script in the page (for the --snapshot test modes).</summary>
    public Task RunScript(string js) => Web.CoreWebView2.ExecuteScriptAsync(js);

    /// <summary>Saves a picture of the page as Edge draws it (for the --snapshot test mode).</summary>
    public async Task CapturePage(string file)
    {
        await using var f = File.Create(file);
        await Web.CoreWebView2.CapturePreviewAsync(CoreWebView2CapturePreviewImageFormat.Png, f);
    }

    void OnAdmin(object sender, RoutedEventArgs e) => _ = OpenAdmin();

    /// <summary>
    /// Opens the admin portal in this window. With a portal session already in this window it goes straight in;
    /// otherwise this PC's administrator signs in: one UAC prompt runs `ppserver admin login-link`, which makes a
    /// one-time link that works for two minutes and only from this machine.
    /// </summary>
    public async Task OpenAdmin(bool forceSignIn = false)
    {
        if (info == null || state != ServiceControllerStatus.Running) return;
        ShowWeb(false);
        for (var i = 0; i < 100 && Web.CoreWebView2 == null; i++) await Task.Delay(100);
        if (Web.CoreWebView2 == null) return;
        var admin = new Uri(info.LocalUrl, "/admin/");
        if (!forceSignIn)
        {
            var cookies = await Web.CoreWebView2.CookieManager.GetCookiesAsync(admin.ToString());
            if (cookies.Any(c => c.Name == "__Host-pp-admin"))
            {
                PageLoaded = false;
                Web.Source = admin;
                return;
            }
        }
        var dir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "DarkCipher Systems", "ParallelPath Server Manager");
        Directory.CreateDirectory(dir);
        var file = Path.Combine(dir, "link-" + Guid.NewGuid().ToString("N") + ".txt");
        try
        {
            var exit = await info.RunAsAdminAndWait("admin", "login-link", "--out", file);
            if (exit == null) return; // the person said no to the prompt
            if (exit != 0 || !File.Exists(file))
            {
                Dialog.Error(this, "Couldn't sign in to the admin portal", "The server didn't make a sign-in link. Check it's running, then try again.");
                return;
            }
            var link = File.ReadAllText(file).Trim();
            var expected = admin + "#local=";
            if (!link.StartsWith(expected, StringComparison.Ordinal)) return;
            PageLoaded = false;
            Web.Source = new Uri(link);
        }
        finally
        {
            try { File.Delete(file); } catch { /* removed on the next try */ }
        }
    }

    void OnMessageAction(object sender, RoutedEventArgs e) => messageAction?.Invoke();

    void OnPower(object sender, RoutedEventArgs e) => Power();

    void Power()
    {
        if (info == null) return;
        info.RunAsAdmin(true, "service", state == ServiceControllerStatus.Running ? "restart" : "start");
    }

    void OnInvite(object sender, RoutedEventArgs e) => info?.RunAsAdmin(false, "invite", "--pause");

    void OnCopy(object sender, RoutedEventArgs e)
    {
        if (info == null) return;
        CopyButton.Content = SafeClipboard.TrySetText(info.FirstAddress) ? "Copied" : "Clipboard busy";
    }

    void OnBrowser(object sender, RoutedEventArgs e) => Browser();

    void Browser()
    {
        if (info != null) ServerInfo.OpenInBrowser(info.LocalUrl);
    }
}
