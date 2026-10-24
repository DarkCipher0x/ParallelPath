using System.Windows;
using System.Windows.Controls;
using PPSetup.Ui;

namespace PPSetup.Server.Pages;

/// <summary>What setup found on this PC, including the dependency check (everything needed is part of Windows).</summary>
public sealed class ChecksPage : WizardPage
{
    readonly SetupState state;
    readonly ProgressList list = new();
    readonly Banner summary = new() { Margin = new Thickness(0, 6, 0, 0) };

    public ChecksPage(SetupState state)
    {
        this.state = state;
        var stack = new StackPanel();
        stack.Children.Add(list);
        stack.Children.Add(summary);
        Content = new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Content = stack };
    }

    public override string Step => "Checks";
    public override string Heading => "Checking this PC";
    public override string Subheading => "Nothing else needs downloading - ParallelPath Server uses only what's built into Windows.";

    bool Blocked => state.Probe.Checks.Any(c => State(c) == RowState.Failed);
    public override bool CanGoNext => !Blocked;

    RowState State(Check c) => c.State switch
    {
        "ok" => RowState.Done,
        "fail" => RowState.Failed,
        "warn" when c.Id == "encryption" && !state.Plan.Unencrypted => RowState.Failed,
        "warn" => RowState.Warn,
        _ => RowState.Info,
    };

    public override void OnShown()
    {
        list.Rows.Clear();
        foreach (var c in state.Probe.Checks)
        {
            var detail = c.Id == "encryption" && c.State != "ok" && state.Plan.Unencrypted ? "Not encrypted - allowed for testing only." : c.Detail;
            list.Rows.Add(new ProgressRow { Id = c.Id, Title = c.Title, Detail = detail, State = State(c) });
        }
        if (Blocked)
        {
            summary.Tone = Tone.Error;
            summary.Text = "Setup can't continue until the problems above are fixed.";
        }
        else
        {
            summary.Tone = Tone.Ok;
            summary.Text = "This PC is ready.";
        }
    }
}
