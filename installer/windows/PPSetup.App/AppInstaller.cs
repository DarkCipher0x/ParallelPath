using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Security.Cryptography;
using Microsoft.Win32;
using PPSetup.Ui;

namespace PPSetup.App;

/// <summary>Installs the ParallelPath app for the current Windows user (no administrator rights needed).</summary>
public static class AppInstaller
{
    const string UninstallKey = @"Software\Microsoft\Windows\CurrentVersion\Uninstall\ParallelPath";
    static readonly Assembly Asm = typeof(AppInstaller).Assembly;

    public static string DefaultDir =>
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "Programs", "DarkCipher Systems", "ParallelPath");

    public static string DataDir => Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "ParallelPath");

    /// <summary>The installed version and folder, if the app is installed.</summary>
    public static (string Version, string Dir)? Existing()
    {
        using var k = Registry.CurrentUser.OpenSubKey(UninstallKey);
        if (k?.GetValue("InstallLocation") is not string dir || !File.Exists(Path.Combine(dir, "ParallelPath.exe"))) return null;
        return ((k.GetValue("DisplayVersion") as string) ?? "?", dir);
    }

    /// <summary>Whether the app (or its engine) is running from <paramref name="dir"/>.</summary>
    public static bool Running(string dir)
    {
        var prefix = Path.GetFullPath(dir).TrimEnd('\\') + "\\";
        foreach (var p in Process.GetProcesses())
        {
            try
            {
                if (p.ProcessName is "ParallelPath" or "ppengine" or "java" &&
                    p.MainModule?.FileName.StartsWith(prefix, StringComparison.OrdinalIgnoreCase) == true)
                    return true;
            }
            catch { /* another user's or a protected process */ }
            finally { p.Dispose(); }
        }
        return false;
    }

    public static long FreeBytes(string dir) => new DriveInfo(Path.GetPathRoot(Path.GetFullPath(dir))!).AvailableFreeSpace;

    /// <summary>Unpacks the app into place: into a new folder first, then swapped in, so a failure leaves the old one intact.</summary>
    public static string InstallFiles(string dir)
    {
        using var zip = Asm.GetManifestResourceStream("app.zip") ?? throw new InvalidDataException("setup is missing the app");
        var want = Payload.ExpectedHash(Asm, "app.zip");
        if (Convert.ToHexString(SHA256.HashData(zip)).ToLowerInvariant() != want)
            throw new InvalidDataException("The app inside setup is damaged (checksum mismatch). Download setup again.");
        zip.Position = 0;
        var fresh = dir + ".new";
        var old = dir + ".old";
        if (Directory.Exists(fresh)) Directory.Delete(fresh, true);
        if (Directory.Exists(old)) Directory.Delete(old, true);
        using (var archive = new ZipArchive(zip, ZipArchiveMode.Read))
            archive.ExtractToDirectory(fresh);
        // The zip holds a single "ParallelPath" folder with the app inside.
        var inner = Path.Combine(fresh, "ParallelPath");
        var root = Directory.Exists(inner) ? inner : fresh;
        Payload.Extract(Asm, "Uninstall ParallelPath.exe", root);
        Directory.CreateDirectory(Path.GetDirectoryName(dir)!);
        if (Directory.Exists(dir)) Directory.Move(dir, old);
        Directory.Move(root, dir);
        if (root != fresh) Directory.Delete(fresh, true);
        if (Directory.Exists(old)) Directory.Delete(old, true);
        return dir;
    }

    public static string Shortcuts(string dir, bool desktop, bool startMenu)
    {
        var made = new List<string>();
        foreach (var (wanted, folder, name) in new[]
                 {
                     (desktop, Environment.SpecialFolder.DesktopDirectory, "Desktop"),
                     (startMenu, Environment.SpecialFolder.Programs, "Start menu"),
                 })
        {
            var path = Path.Combine(Environment.GetFolderPath(folder), "ParallelPath.lnk");
            File.Delete(path);
            if (!wanted) continue;
            var shell = Activator.CreateInstance(Type.GetTypeFromProgID("WScript.Shell")!)!;
            dynamic link = ((dynamic)shell).CreateShortcut(path);
            link.TargetPath = Path.Combine(dir, "ParallelPath.exe");
            link.WorkingDirectory = dir;
            link.Description = "ParallelPath - post-quantum secure co-parenting";
            link.IconLocation = Path.Combine(dir, "ParallelPath.exe") + ",0";
            link.Save();
            made.Add(name);
        }
        return made.Count == 0 ? "none" : string.Join(" and ", made);
    }

    public static string Register(string dir, string version)
    {
        using var k = Registry.CurrentUser.CreateSubKey(UninstallKey);
        var uninstaller = Path.Combine(dir, "Uninstall ParallelPath.exe");
        k.SetValue("DisplayName", "ParallelPath");
        k.SetValue("DisplayVersion", version);
        k.SetValue("Publisher", "DarkCipher Systems");
        k.SetValue("Comments", "Post-quantum secure co-parenting");
        k.SetValue("DisplayIcon", Path.Combine(dir, "ParallelPath.exe") + ",0");
        k.SetValue("InstallLocation", dir);
        k.SetValue("UninstallString", "\"" + uninstaller + "\"");
        k.SetValue("QuietUninstallString", "\"" + uninstaller + "\" --quiet");
        k.SetValue("NoModify", 1, RegistryValueKind.DWord);
        k.SetValue("NoRepair", 1, RegistryValueKind.DWord);
        k.SetValue("EstimatedSize", (int)(DirSize(dir) / 1024), RegistryValueKind.DWord);
        return "Settings › Apps › ParallelPath " + version;
    }

    static long DirSize(string dir) => new DirectoryInfo(dir).EnumerateFiles("*", SearchOption.AllDirectories).Sum(f => f.Length);

    public static void Launch(string dir) =>
        Process.Start(new ProcessStartInfo(Path.Combine(dir, "ParallelPath.exe")) { UseShellExecute = true, WorkingDirectory = dir });
}
