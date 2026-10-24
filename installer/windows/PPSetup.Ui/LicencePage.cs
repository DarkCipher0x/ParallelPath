using System.IO;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;

namespace PPSetup.Ui;

/// <summary>The AGPL-3.0 licence, which must be accepted before anything is installed.</summary>
public sealed class LicencePage : WizardPage
{
    readonly CheckBox accept = new() { Content = "I accept the licence", Margin = new Thickness(0, 14, 0, 0) };

    public LicencePage()
    {
        using var stream = typeof(LicencePage).Assembly.GetManifestResourceStream("LICENSE")!;
        var text = new StreamReader(stream).ReadToEnd();
        var body = new TextBox
        {
            Text = "ParallelPath is free and open-source software, built by DarkCipher0x at DarkCipher Systems.\n" +
                   "You may use, study, share and change it under the GNU Affero General Public License, version 3.\n\n" + text,
            IsReadOnly = true, TextWrapping = TextWrapping.Wrap, VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
            FontSize = 12.5, Padding = new Thickness(14, 12, 14, 12),
        };
        Loaded += (_, _) => body.FontFamily = (FontFamily)FindResource("MonoFont");
        accept.Checked += (_, _) => Changed();
        accept.Unchecked += (_, _) => Changed();
        var dock = new DockPanel();
        DockPanel.SetDock(accept, Dock.Bottom);
        dock.Children.Add(accept);
        dock.Children.Add(body);
        Content = dock;
    }

    public override string Step => "Licence";
    public override string Heading => "Licence";
    public override string Subheading => "Please read and accept the licence to continue.";
    public override bool CanGoNext => accept.IsChecked == true;

    /// <summary>When to leave the page out (an upgrade has already accepted the licence).</summary>
    public Func<bool>? SkipIf { get; init; }
    public override bool Skip => SkipIf?.Invoke() ?? false;

    /// <summary>For automated tests.</summary>
    public void Accept() => accept.IsChecked = true;
}
