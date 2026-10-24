using System.Windows;
using System.Windows.Controls;
using PPSetup.Ui;

namespace PPSetup.Server.Pages;

/// <summary>Runs the installation and shows each step as ppserver reports it.</summary>
public sealed class InstallPage : WizardPage
{
    readonly SetupState state;
    readonly ProgressList list = new();
    readonly StackPanel notes = new();
    readonly Banner failure = new() { Tone = Tone.Error, Visibility = Visibility.Collapsed, Margin = new Thickness(0, 0, 0, 14) };
    bool running, failed;

    public InstallPage(SetupState state)
    {
        this.state = state;
        var stack = new StackPanel();
        stack.Children.Add(failure);
        stack.Children.Add(list);
        stack.Children.Add(notes);
        Content = new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Content = stack };
    }

    public override string Step => "Install";
    public override string Heading => failed ? "Setup didn't finish" : state.Upgrade ? "Upgrading…" : "Installing…";
    public override string Subheading => failed ? "Nothing you've already set up is lost. Fix the problem below, then go Back and try again." : "";
    public override bool ShowNext => failed;
    public override string NextText => "Close";
    public override bool CanGoBack => failed;
    public override bool CanCancel => false;
    public override bool CanClose => !running;
    public override bool AskBeforeClosing => false;

    public bool Finished { get; private set; }

    public override async void OnShown()
    {
        if (running) return;
        running = true;
        failed = false;
        failure.Visibility = Visibility.Collapsed;
        list.Rows.Clear();
        notes.Children.Clear();
        Changed();
        var plan = state.Plan;
        if (state.Upgrade) plan = new Plan { Mode = "upgrade", DataDir = state.Probe.Existing!.DataDir };
        // This program is also ParallelPath Server Manager: setup installs a copy of itself as the icon by the clock.
        plan.ManagerExe = Environment.ProcessPath!;
        try
        {
            state.Result = await state.Tool.InstallAsync(plan, OnEvent);
            running = false;
            Finished = true;
            Wizard.Forward();
        }
        catch (Exception ex)
        {
            running = false;
            failed = true;
            failure.Text = ex.Message;
            failure.Visibility = Visibility.Visible;
            Changed();
        }
    }

    void OnEvent(InstallEvent e)
    {
        var row = list.Rows.FirstOrDefault(r => r.Id == e.Id);
        switch (e.Type)
        {
            case "step":
                list.Rows.Add(new ProgressRow { Id = e.Id, Title = e.Title, State = RowState.Running });
                break;
            case "done" when row != null:
                row.State = RowState.Done;
                row.Detail = e.Detail;
                break;
            case "skip":
                list.Rows.Add(new ProgressRow { Id = e.Id, Title = e.Title, Detail = e.Reason, State = RowState.Skipped });
                break;
            case "fail" when row != null:
                row.State = RowState.Failed;
                row.Detail = e.Error;
                break;
            case "note":
                notes.Children.Add(new Banner { Tone = Tone.Warn, Text = e.Text, Margin = new Thickness(0, 4, 0, 8) });
                break;
        }
    }

    public override Task<bool> OnNextAsync()
    {
        Wizard.Close();
        return Task.FromResult(false);
    }
}
