using System.Windows;
using PPSetup.Ui;

namespace PPSetup.Server.Pages;

public partial class WelcomePage : WizardPage
{
    readonly SetupState state;
    bool probed, probing;

    public WelcomePage(SetupState state)
    {
        this.state = state;
        InitializeComponent();
    }

    public override string Step => "Welcome";
    public override string Heading => state.Upgrade ? "Upgrade ParallelPath Server" : "Welcome to ParallelPath Server";
    public override string Subheading => "Post-quantum secure co-parenting, hosted by you.";
    public override bool CanGoNext => probed;
    public override bool CanGoBack => false;

    /// <summary>For screenshots and tests: use a probe that's already been made.</summary>
    public void UseProbe(Probe p)
    {
        Apply(p);
        probed = true;
    }

    public override async void OnShown()
    {
        if (probed || probing) return;
        probing = true;
        try
        {
            Apply(await state.Tool.ProbeAsync());
            probed = true;
        }
        catch (Exception ex)
        {
            Notice.Tone = Tone.Error;
            Notice.Text = ex.Message;
            Notice.Visibility = Visibility.Visible;
            Status.Text = "";
        }
        probing = false;
        Changed();
    }

    void Apply(Probe p)
    {
        state.Probe = p;
        state.Plan = p.Defaults;
        Status.Text = $"Setup version {p.Version} · {p.Hostname}";
        if (p.Existing is { } e)
        {
            Notice.Tone = Tone.Info;
            var action = e.Version == p.Version ? $"repair it (reinstall {p.Version})" : $"upgrade it to {p.Version}";
            Notice.Text = $"ParallelPath Server {e.Version} is installed here ({e.State}). Setup will {action}, " +
                          "keeping all data, settings and keys. The server restarts once.";
            Notice.Visibility = Visibility.Visible;
        }
        else if (p.Checks.Any(c => c.Id == "previous"))
        {
            Notice.Tone = Tone.Warn;
            Notice.Text = "An earlier installation didn't finish. Setup will complete it.";
            Notice.Visibility = Visibility.Visible;
        }
    }
}
