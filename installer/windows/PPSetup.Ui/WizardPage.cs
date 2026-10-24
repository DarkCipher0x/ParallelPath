using System.Windows.Controls;

namespace PPSetup.Ui;

/// <summary>One page of the setup wizard. Override what the page needs; the defaults suit an ordinary page.</summary>
public abstract class WizardPage : UserControl
{
    public WizardWindow Wizard { get; internal set; } = null!;

    /// <summary>The step name in the rail (several pages can share one).</summary>
    public abstract string Step { get; }
    public abstract string Heading { get; }
    public virtual string Subheading => "";

    public virtual string NextText => "Next";
    public virtual bool ShowNext => true;
    public virtual bool CanGoNext => true;
    public virtual bool CanGoBack => true;
    public virtual bool CanCancel => true;
    /// <summary>False while the page is changing the system (closing would leave things half done).</summary>
    public virtual bool CanClose => true;
    /// <summary>Whether closing should be confirmed (true until anything has been changed).</summary>
    public virtual bool AskBeforeClosing => true;
    /// <summary>Pages that don't apply (an upgrade skips the settings pages).</summary>
    public virtual bool Skip => false;

    /// <summary>Runs when Next is pressed; return false to stay on the page.</summary>
    public virtual Task<bool> OnNextAsync() => Task.FromResult(true);

    /// <summary>Runs each time the page is shown.</summary>
    public virtual void OnShown() { }

    /// <summary>Tells the wizard the page's state (e.g. whether Next is allowed) has changed.</summary>
    protected void Changed() => Wizard?.Refresh();
}
