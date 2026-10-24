using System.Text.Json.Serialization;

namespace PPSetup.Server;

// These mirror server/internal/install (plan, probe, result and progress events) field for field.

public sealed class Plan
{
    [JsonPropertyName("mode")] public string Mode { get; set; } = "install";
    [JsonPropertyName("install_dir")] public string InstallDir { get; set; } = "";
    [JsonPropertyName("data_dir")] public string DataDir { get; set; } = "";
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("hosts")] public List<string> Hosts { get; set; } = new();
    [JsonPropertyName("port")] public int Port { get; set; } = 8443;
    [JsonPropertyName("autostart")] public bool AutoStart { get; set; } = true;
    [JsonPropertyName("firewall")] public bool Firewall { get; set; } = true;
    [JsonPropertyName("firewall_lan_only")] public bool LanOnly { get; set; } = true;
    [JsonPropertyName("backups")] public bool Backups { get; set; } = true;
    [JsonPropertyName("backup_dir")] public string BackupDir { get; set; } = "";
    [JsonPropertyName("backup_keep")] public int BackupKeep { get; set; } = 14;
    [JsonPropertyName("shortcuts")] public bool Shortcuts { get; set; } = true;
    [JsonPropertyName("encrypted_container")] public bool Container { get; set; }
    [JsonPropertyName("container_gb")] public int ContainerGb { get; set; }
    [JsonPropertyName("install_packages")] public bool Packages { get; set; }
    [JsonPropertyName("allow_unencrypted")] public bool Unencrypted { get; set; }
    [JsonPropertyName("first_invite")] public bool FirstInvite { get; set; } = true;
    [JsonPropertyName("tray")] public bool Tray { get; set; } = true;
    [JsonPropertyName("manager_exe")] public string ManagerExe { get; set; } = "";
    [JsonPropertyName("tls_mode")] public string TlsMode { get; set; } = "local-ca";
    [JsonPropertyName("acme_domain")] public string AcmeDomain { get; set; } = "";
    [JsonPropertyName("acme_email")] public string AcmeEmail { get; set; } = "";
    [JsonPropertyName("cert_file")] public string CertFile { get; set; } = "";
    [JsonPropertyName("key_file")] public string KeyFile { get; set; } = "";
    [JsonPropertyName("trust_ca")] public bool TrustCa { get; set; } = true;
    [JsonPropertyName("calls")] public bool Calls { get; set; } = true;
}

public sealed class Check
{
    [JsonPropertyName("id")] public string Id { get; set; } = "";
    [JsonPropertyName("title")] public string Title { get; set; } = "";
    [JsonPropertyName("state")] public string State { get; set; } = "";
    [JsonPropertyName("detail")] public string Detail { get; set; } = "";
}

public sealed class Existing
{
    [JsonPropertyName("version")] public string Version { get; set; } = "";
    [JsonPropertyName("install_dir")] public string InstallDir { get; set; } = "";
    [JsonPropertyName("data_dir")] public string DataDir { get; set; } = "";
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("state")] public string State { get; set; } = "";
}

public sealed class Probe
{
    [JsonPropertyName("os")] public string Os { get; set; } = "";
    [JsonPropertyName("version")] public string Version { get; set; } = "";
    [JsonPropertyName("admin")] public bool Admin { get; set; }
    [JsonPropertyName("hostname")] public string Hostname { get; set; } = "";
    [JsonPropertyName("addresses")] public List<string>? Addresses { get; set; }
    [JsonPropertyName("existing")] public Existing? Existing { get; set; }
    [JsonPropertyName("checks")] public List<Check> Checks { get; set; } = new();
    [JsonPropertyName("defaults")] public Plan Defaults { get; set; } = new();

    public Check? Find(string id) => Checks.FirstOrDefault(c => c.Id == id);
    public bool Encrypted => Find("encryption")?.State is null or "ok";
}

public sealed class Result
{
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("version")] public string Version { get; set; } = "";
    [JsonPropertyName("server_key")] public string ServerKey { get; set; } = "";
    [JsonPropertyName("tls_cert_sha256")] public string CertSha256 { get; set; } = "";
    [JsonPropertyName("addresses")] public List<string>? Addresses { get; set; }
    [JsonPropertyName("at_rest")] public string AtRest { get; set; } = "";
    [JsonPropertyName("invite")] public string? Invite { get; set; }
    [JsonPropertyName("invite_expires")] public string? InviteExpires { get; set; }
    [JsonPropertyName("data_dir")] public string DataDir { get; set; } = "";
    [JsonPropertyName("logs")] public string Logs { get; set; } = "";
    [JsonPropertyName("notes")] public List<string>? Notes { get; set; }
}

/// <summary>One line of `ppserver install --json` output.</summary>
public sealed class InstallEvent
{
    [JsonPropertyName("type")] public string Type { get; set; } = "";
    [JsonPropertyName("id")] public string Id { get; set; } = "";
    [JsonPropertyName("title")] public string Title { get; set; } = "";
    [JsonPropertyName("detail")] public string Detail { get; set; } = "";
    [JsonPropertyName("reason")] public string Reason { get; set; } = "";
    [JsonPropertyName("error")] public string Error { get; set; } = "";
    [JsonPropertyName("text")] public string Text { get; set; } = "";
    [JsonPropertyName("result")] public Result? Result { get; set; }
}

/// <summary>Everything the pages share.</summary>
public sealed class SetupState
{
    public required ServerTool Tool { get; init; }
    public Probe Probe { get; set; } = new();
    public Plan Plan { get; set; } = new();
    public Result? Result { get; set; }
    public bool Upgrade => Probe.Existing != null;
}
