using System.IO;
using System.Reflection;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;

namespace PPSetup.Ui;

/// <summary>The programs a setup carries inside itself, unpacked safely before they run.</summary>
public static class Payload
{
    /// <summary>The SHA-256 (hex) recorded for an embedded file at build time (resource "name.sha256").</summary>
    public static string ExpectedHash(Assembly asm, string name)
    {
        using var s = asm.GetManifestResourceStream(name + ".sha256") ?? throw new InvalidDataException("setup is missing " + name + ".sha256");
        return new StreamReader(s).ReadToEnd().Trim().Split(' ')[0].ToLowerInvariant();
    }

    /// <summary>
    /// Writes an embedded file into a new folder under ProgramData that only SYSTEM and Administrators can change, then
    /// checks its SHA-256 against the one recorded at build time. Because nobody else can write there (or rename the
    /// folder), the file that runs elevated is exactly the one that was checked.
    /// </summary>
    public static string ExtractForAdmin(Assembly asm, string name)
    {
        var dir = new DirectoryInfo(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData),
            "ParallelPath-setup-" + Guid.NewGuid().ToString("N")));
        var sec = new DirectorySecurity();
        sec.SetAccessRuleProtection(true, false);
        foreach (var sid in new[] { WellKnownSidType.LocalSystemSid, WellKnownSidType.BuiltinAdministratorsSid })
            sec.AddAccessRule(new FileSystemAccessRule(new SecurityIdentifier(sid, null), FileSystemRights.FullControl,
                InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit, PropagationFlags.None, AccessControlType.Allow));
        sec.SetOwner(new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null));
        dir.Create(sec);
        return Extract(asm, name, dir.FullName);
    }

    /// <summary>Writes an embedded file into <paramref name="dir"/> and checks its SHA-256.</summary>
    public static string Extract(Assembly asm, string name, string dir)
    {
        var path = Path.Combine(dir, name);
        using (var src = asm.GetManifestResourceStream(name) ?? throw new InvalidDataException("setup is missing " + name))
        using (var dst = new FileStream(path, FileMode.CreateNew, FileAccess.Write))
            src.CopyTo(dst);
        var want = ExpectedHash(asm, name);
        if (Sha256(path) != want)
        {
            File.Delete(path);
            throw new InvalidDataException(name + " inside setup is damaged (checksum mismatch). Download setup again.");
        }
        return path;
    }

    public static string Sha256(string path)
    {
        using var f = File.OpenRead(path);
        return Convert.ToHexString(SHA256.HashData(f)).ToLowerInvariant();
    }

    /// <summary>Removes an extraction folder (best effort).</summary>
    public static void Cleanup(string file)
    {
        try { Directory.Delete(Path.GetDirectoryName(file)!, true); } catch { /* left for the next reboot clean-up */ }
    }
}
