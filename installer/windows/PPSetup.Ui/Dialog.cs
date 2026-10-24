using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;

namespace PPSetup.Ui;

/// <summary>Message boxes in the ParallelPath style (the system ones would break the look).</summary>
public static class Dialog
{
    public static void Info(Window owner, string title, string message) => Show(owner, title, message, "OK", null, "Accent");

    public static void Error(Window owner, string title, string message) => Show(owner, title, message, "OK", null, "Danger");

    public static bool Confirm(Window owner, string title, string message, string yes, string no) =>
        Show(owner, title, message, yes, no, "Warm");

    static bool Show(Window owner, string title, string message, string yes, string? no, string tone)
    {
        var res = owner.Resources;
        var w = new Window
        {
            Owner = owner, Title = title, Width = 460, SizeToContent = SizeToContent.Height, ResizeMode = ResizeMode.NoResize,
            WindowStartupLocation = WindowStartupLocation.CenterOwner, ShowInTaskbar = false,
            Background = (Brush)res["Panel"], Foreground = (Brush)res["Fg"], FontFamily = (FontFamily)res["UiFont"], FontSize = 14,
        };
        w.SourceInitialized += (_, _) => Native.DarkTitleBar(w);
        var result = false;
        var stack = new StackPanel { Margin = new Thickness(26, 22, 26, 20) };
        stack.Children.Add(new Border { Height = 3, Width = 40, HorizontalAlignment = HorizontalAlignment.Left, CornerRadius = new CornerRadius(2),
            Background = (Brush)res[tone], Margin = new Thickness(0, 0, 0, 14) });
        stack.Children.Add(new TextBlock { Text = title, Style = (Style)res["H2"], FontSize = 17 });
        stack.Children.Add(new TextBlock { Text = message, Style = (Style)res["Body"], Margin = new Thickness(0, 8, 0, 20) });
        var buttons = new StackPanel { Orientation = Orientation.Horizontal, HorizontalAlignment = HorizontalAlignment.Right };
        if (no != null)
        {
            var noButton = new Button { Content = no, Style = (Style)res["Secondary"], Margin = new Thickness(0, 0, 10, 0), IsCancel = true };
            noButton.Click += (_, _) => w.Close();
            buttons.Children.Add(noButton);
        }
        var yesButton = new Button { Content = yes, Style = (Style)res["Primary"], IsDefault = true };
        yesButton.Click += (_, _) => { result = true; w.Close(); };
        buttons.Children.Add(yesButton);
        stack.Children.Add(buttons);
        w.Content = stack;
        w.ShowDialog();
        return result;
    }
}
