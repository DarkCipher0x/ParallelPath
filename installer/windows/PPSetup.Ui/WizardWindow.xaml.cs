using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Windows;

namespace PPSetup.Ui;

/// <summary>
/// The setup wizard: the brand rail with its steps on the left, the current page on the right, and Back / Next /
/// Cancel underneath. Pages decide what Next means (validate, start work) through <see cref="WizardPage"/>.
/// </summary>
public partial class WizardWindow : Window
{
    readonly List<WizardPage> pages;
    readonly ObservableCollection<StepView> steps = new();
    int index = -1;
    bool busy;

    public WizardWindow(string productLine, string version, IEnumerable<WizardPage> pages)
    {
        InitializeComponent();
        Title = "ParallelPath " + productLine;
        ProductLine.Text = productLine;
        VersionLine.Text = "Version " + version;
        this.pages = pages.ToList();
        StepList.ItemsSource = steps;
        SourceInitialized += (_, _) => Native.DarkTitleBar(this);
        Loaded += (_, _) => GoTo(0);
    }

    public WizardPage Current => pages[index];

    /// <summary>Shows the page at <paramref name="i"/>.</summary>
    public void GoTo(int i)
    {
        index = i;
        var page = pages[i];
        page.Wizard = this;
        PageHost.Content = page;
        Refresh();
        page.OnShown();
        PageShown?.Invoke(page);
    }

    /// <summary>Shows a specific page.</summary>
    public void GoTo(WizardPage page) => GoTo(pages.IndexOf(page));

    /// <summary>Re-reads the current page's state into the header, the buttons and the step list.</summary>
    public void Refresh()
    {
        var page = Current;
        Heading.Text = page.Heading;
        Subheading.Text = page.Subheading;
        Subheading.Visibility = string.IsNullOrEmpty(page.Subheading) ? Visibility.Collapsed : Visibility.Visible;
        NextButton.Content = page.NextText;
        NextButton.IsEnabled = page.CanGoNext && !busy;
        NextButton.Visibility = page.ShowNext ? Visibility.Visible : Visibility.Collapsed;
        BackButton.Visibility = page.CanGoBack && Previous() >= 0 ? Visibility.Visible : Visibility.Collapsed;
        BackButton.IsEnabled = !busy;
        CancelButton.Visibility = page.CanCancel ? Visibility.Visible : Visibility.Collapsed;
        CancelButton.IsEnabled = !busy;

        var shown = pages.Where(p => !p.Skip).Select(p => p.Step).Distinct().ToList();
        var currentStep = shown.IndexOf(page.Step);
        steps.Clear();
        for (var s = 0; s < shown.Count; s++)
            steps.Add(new StepView(shown[s], s < currentStep ? StepState.Done : s == currentStep ? StepState.Current : StepState.Upcoming));
    }

    int Previous()
    {
        var i = index - 1;
        while (i >= 0 && pages[i].Skip) i--;
        return i;
    }

    /// <summary>Moves to the next page that isn't skipped, or closes the window after the last one.</summary>
    public void Forward()
    {
        var i = index + 1;
        while (i < pages.Count && pages[i].Skip) i++;
        if (i >= pages.Count) Close();
        else GoTo(i);
    }

    /// <summary>Raised after a page is shown (used by the screenshot and test modes).</summary>
    public event Action<WizardPage>? PageShown;

    void OnNext(object sender, RoutedEventArgs e) => _ = PressNextAsync();

    /// <summary>Does what pressing Next does.</summary>
    public async Task PressNextAsync()
    {
        busy = true;
        Refresh();
        var ok = false;
        try
        {
            ok = await Current.OnNextAsync();
        }
        catch (Exception ex)
        {
            Dialog.Error(this, "Something went wrong", ex.Message);
        }
        finally
        {
            busy = false;
        }
        if (ok) Forward();
        else Refresh();
    }

    void OnBack(object sender, RoutedEventArgs e)
    {
        var i = Previous();
        if (i >= 0) GoTo(i);
    }

    void OnCancel(object sender, RoutedEventArgs e) => Close();

    protected override void OnClosing(CancelEventArgs e)
    {
        base.OnClosing(e);
        if (index < 0) return;
        if (!Current.CanClose)
        {
            e.Cancel = true;
            Dialog.Info(this, "Please wait", "Setup is making changes. It can't be stopped halfway - it will finish in a moment.");
            return;
        }
        if (Current.AskBeforeClosing && !Dialog.Confirm(this, "Cancel setup?", "Nothing has been changed yet. You can run setup again at any time.",
                "Cancel setup", "Keep going"))
            e.Cancel = true;
    }
}

public enum StepState { Upcoming, Current, Done }

public sealed record StepView(string Title, StepState State);
