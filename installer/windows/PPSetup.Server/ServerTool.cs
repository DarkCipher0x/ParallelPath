using System.Diagnostics;
using System.IO;
using System.Text;
using System.Text.Json;
using PPSetup.Ui;

namespace PPSetup.Server;

/// <summary>
/// Runs the ppserver.exe carried inside this setup. All the real installing is done by ppserver itself (the same code
/// the Linux installer uses); this app only gathers the choices and shows the progress.
/// </summary>
public sealed class ServerTool : IDisposable
{
    public string Exe { get; }

    ServerTool(string exe) => Exe = exe;

    /// <summary>Unpacks ppserver.exe into an administrators-only folder and checks its checksum.</summary>
    public static ServerTool Unpack() => new(Payload.ExtractForAdmin(typeof(ServerTool).Assembly, "ppserver.exe"));

    /// <summary>For --screenshots: no program is run.</summary>
    public static ServerTool None() => new("");

    public void Dispose()
    {
        if (Exe != "") Payload.Cleanup(Exe);
    }

    ProcessStartInfo Start(params string[] args)
    {
        var psi = new ProcessStartInfo(Exe)
        {
            UseShellExecute = false, CreateNoWindow = true, RedirectStandardInput = true, RedirectStandardOutput = true,
            RedirectStandardError = true, StandardOutputEncoding = Encoding.UTF8, StandardErrorEncoding = Encoding.UTF8,
            StandardInputEncoding = new UTF8Encoding(false), WorkingDirectory = Path.GetDirectoryName(Exe)!,
        };
        foreach (var a in args) psi.ArgumentList.Add(a);
        return psi;
    }

    /// <summary>Looks at this PC for the chosen data folder and port.</summary>
    public async Task<Probe> ProbeAsync(string? dataDir = null, int port = 0)
    {
        var args = new List<string> { "install", "--probe" };
        if (!string.IsNullOrEmpty(dataDir)) args.AddRange(["--data", dataDir]);
        if (port > 0) args.AddRange(["--port", port.ToString()]);
        using var p = Process.Start(Start(args.ToArray()))!;
        p.StandardInput.Close();
        var stdout = await p.StandardOutput.ReadToEndAsync();
        var stderr = await p.StandardError.ReadToEndAsync();
        await p.WaitForExitAsync();
        if (p.ExitCode != 0) throw new InvalidOperationException("Couldn't check this PC: " + stderr.Trim());
        return JsonSerializer.Deserialize<Probe>(stdout) ?? throw new InvalidDataException("unexpected answer from ppserver");
    }

    /// <summary>Installs (or upgrades) from the plan, reporting each progress event on the calling thread.</summary>
    public async Task<Result> InstallAsync(Plan plan, Action<InstallEvent> onEvent)
    {
        using var p = Process.Start(Start("install", "--plan", "-", "--json"))!;
        await p.StandardInput.WriteAsync(JsonSerializer.Serialize(plan));
        p.StandardInput.Close();
        var stderr = p.StandardError.ReadToEndAsync();
        Result? result = null;
        string? error = null;
        while (await p.StandardOutput.ReadLineAsync() is { } line)
        {
            if (line.Trim().Length == 0) continue;
            InstallEvent? ev;
            try { ev = JsonSerializer.Deserialize<InstallEvent>(line); }
            catch (JsonException) { continue; }
            if (ev == null) continue;
            if (ev.Type == "result") result = ev.Result;
            if (ev.Type == "error") error = ev.Error;
            onEvent(ev);
        }
        await p.WaitForExitAsync();
        if (result != null && p.ExitCode == 0) return result;
        var err = (await stderr).Trim();
        throw new InvalidOperationException(error ?? (err.Length > 0 ? err : "setup stopped unexpectedly (exit code " + p.ExitCode + ")"));
    }
}
