using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.ServiceProcess;
using Microsoft.Win32;

namespace PPSetup.Server.Manager;

/// <summary>
/// What the Manager knows about the installed server: the public details setup recorded in the registry (the data
/// folder itself is for administrators only), and the service's state, which any account may read.
/// </summary>
public sealed record ServerInfo(string InstallDir, string Name, string Version, int Port, string CertSha256, IReadOnlyList<string> Addresses)
{
    const string ProductKey = @"SOFTWARE\DarkCipher Systems\ParallelPath Server";
    public const string ServiceName = "ParallelPathServer";

    public string Ppserver => Path.Combine(InstallDir, "ppserver.exe");

    /// <summary>The server's own page, over loopback (whatever names its certificate covers, the Manager pins it).</summary>
    public Uri LocalUrl => new($"https://127.0.0.1:{Port}/");

    public string FirstAddress => Addresses.FirstOrDefault() ?? $"127.0.0.1:{Port}";

    public static ServerInfo? Load()
    {
        using var hklm = RegistryKey.OpenBaseKey(RegistryHive.LocalMachine, RegistryView.Registry64);
        using var k = hklm.OpenSubKey(ProductKey);
        if (k?.GetValue("InstallDir") is not string dir) return null;
        return new ServerInfo(dir, k.GetValue("Name") as string ?? "ParallelPath Server", k.GetValue("Version") as string ?? "",
            k.GetValue("Port") is int port ? port : 8443, (k.GetValue("CertSHA256") as string ?? "").ToLowerInvariant(),
            k.GetValue("Addresses") as string[] ?? []);
    }

    /// <summary>The service's state, or null when it isn't installed.</summary>
    public static ServiceControllerStatus? State()
    {
        try
        {
            using var s = new ServiceController(ServiceName);
            return s.Status;
        }
        catch (InvalidOperationException)
        {
            return null;
        }
    }

    /// <summary>
    /// Runs a ppserver command as administrator (one UAC prompt). Commands that print something keep their console
    /// window open (--pause); service control runs hidden. Returns false if the person said no to the prompt.
    /// </summary>
    public bool RunAsAdmin(bool hidden, params string[] args)
    {
        var psi = new ProcessStartInfo(Ppserver) { UseShellExecute = true, Verb = "runas", WorkingDirectory = InstallDir,
            WindowStyle = hidden ? ProcessWindowStyle.Hidden : ProcessWindowStyle.Normal };
        foreach (var a in args) psi.ArgumentList.Add(a);
        try
        {
            Process.Start(psi)?.Dispose();
            return true;
        }
        catch (Win32Exception ex) when (ex.NativeErrorCode == 1223) // the UAC prompt was cancelled
        {
            return false;
        }
    }

    /// <summary>Runs a ppserver command as administrator, hidden, and waits for it. Null if the person said no.</summary>
    public async Task<int?> RunAsAdminAndWait(params string[] args)
    {
        var psi = new ProcessStartInfo(Ppserver) { UseShellExecute = true, Verb = "runas", WorkingDirectory = InstallDir,
            WindowStyle = ProcessWindowStyle.Hidden };
        foreach (var a in args) psi.ArgumentList.Add(a);
        try
        {
            using var p = Process.Start(psi);
            if (p == null) return null;
            await p.WaitForExitAsync();
            return p.ExitCode;
        }
        catch (Win32Exception ex) when (ex.NativeErrorCode == 1223)
        {
            return null;
        }
    }

    public static void OpenInBrowser(Uri url) => Process.Start(new ProcessStartInfo(url.ToString()) { UseShellExecute = true })?.Dispose();
}
