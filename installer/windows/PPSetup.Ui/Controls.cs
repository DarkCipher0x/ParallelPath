using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Media.Animation;
using System.Windows.Shapes;

namespace PPSetup.Ui;

public enum Tone { Info, Ok, Warn, Error }

/// <summary>An inline message: information, success, warning or error.</summary>
public sealed class Banner : Border
{
    readonly TextBlock text = new() { TextWrapping = TextWrapping.Wrap, FontSize = 13.5, LineHeight = 20 };
    readonly Border stripe = new() { Width = 3, CornerRadius = new CornerRadius(2), Margin = new Thickness(0, 0, 12, 0) };

    public Banner()
    {
        CornerRadius = new CornerRadius(10);
        Padding = new Thickness(14, 12, 14, 12);
        BorderThickness = new Thickness(1);
        var dock = new DockPanel();
        DockPanel.SetDock(stripe, Dock.Left);
        dock.Children.Add(stripe);
        dock.Children.Add(text);
        Child = dock;
        Loaded += (_, _) => Apply();
    }

    public string Text { get => text.Text; set => text.Text = value; }

    Tone tone;
    public Tone Tone { get => tone; set { tone = value; Apply(); } }

    void Apply()
    {
        if (TryFindResource("Fg") is not Brush fg) return;
        var (accent, soft) = tone switch
        {
            Tone.Ok => ("Ok", "OkSoft"),
            Tone.Warn => ("Warm", "WarmSoft"),
            Tone.Error => ("Danger", "DangerSoft"),
            _ => ("Accent", "AccentSoft"),
        };
        var color = ((SolidColorBrush)FindResource(accent)).Color;
        stripe.Background = new SolidColorBrush(color);
        BorderBrush = new SolidColorBrush(Color.FromArgb(0x70, color.R, color.G, color.B));
        Background = (Brush)FindResource(soft);
        text.Foreground = fg;
    }
}

public enum RowState { Waiting, Running, Done, Skipped, Failed, Info, Warn }

/// <summary>One line of a progress or checks list.</summary>
public sealed class ProgressRow : INotifyPropertyChanged
{
    string title = "", detail = "";
    RowState state;

    public string Id { get; init; } = "";
    public string Title { get => title; set => Set(ref title, value); }
    public string Detail { get => detail; set => Set(ref detail, value); }
    public RowState State { get => state; set => Set(ref state, value); }

    public event PropertyChangedEventHandler? PropertyChanged;

    void Set<T>(ref T field, T value, [CallerMemberName] string name = "")
    {
        field = value;
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
    }
}

/// <summary>A list of steps or checks, each with a state mark (spinner while running).</summary>
public sealed class ProgressList : ItemsControl
{
    public ObservableCollection<ProgressRow> Rows { get; } = new();

    public ProgressList()
    {
        ItemsSource = Rows;
        Focusable = false;
        ItemTemplate = new DataTemplate { VisualTree = new FrameworkElementFactory(typeof(ProgressRowView)) };
    }
}

/// <summary>Draws one <see cref="ProgressRow"/>.</summary>
public sealed class ProgressRowView : Grid
{
    readonly Grid mark = new() { Width = 20, Height = 20, VerticalAlignment = VerticalAlignment.Top, Margin = new Thickness(0, 1, 12, 0) };
    readonly TextBlock title = new() { FontSize = 14, TextWrapping = TextWrapping.Wrap };
    readonly TextBlock detail = new() { FontSize = 12.5, TextWrapping = TextWrapping.Wrap, LineHeight = 18, Margin = new Thickness(0, 2, 0, 0) };

    public ProgressRowView()
    {
        Margin = new Thickness(0, 0, 0, 12);
        ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        ColumnDefinitions.Add(new ColumnDefinition());
        var texts = new StackPanel();
        texts.Children.Add(title);
        texts.Children.Add(detail);
        SetColumn(texts, 1);
        Children.Add(mark);
        Children.Add(texts);
        DataContextChanged += (_, e) =>
        {
            if (e.OldValue is ProgressRow old) old.PropertyChanged -= Changed;
            if (e.NewValue is ProgressRow row) row.PropertyChanged += Changed;
            Draw();
        };
    }

    void Changed(object? sender, PropertyChangedEventArgs e) => Dispatcher.Invoke(Draw);

    Brush B(string key) => (Brush)FindResource(key);

    void Draw()
    {
        if (DataContext is not ProgressRow row) return;
        title.Text = row.Title;
        detail.Text = row.Detail;
        detail.Visibility = string.IsNullOrEmpty(row.Detail) ? Visibility.Collapsed : Visibility.Visible;
        title.Foreground = row.State is RowState.Waiting or RowState.Skipped ? B("FgMute") : B("Fg");
        detail.Foreground = row.State == RowState.Failed ? B("Danger") : B("FgMute");
        mark.Children.Clear();
        switch (row.State)
        {
            case RowState.Running:
                var arc = new Path
                {
                    Data = Geometry.Parse("M 10,2 A 8,8 0 1 1 2,10"), Stroke = B("Accent"), StrokeThickness = 2.2,
                    StrokeStartLineCap = PenLineCap.Round, StrokeEndLineCap = PenLineCap.Round,
                    RenderTransformOrigin = new Point(0.5, 0.5), RenderTransform = new RotateTransform(),
                };
                arc.RenderTransform.BeginAnimation(RotateTransform.AngleProperty,
                    new DoubleAnimation(0, 360, TimeSpan.FromSeconds(0.9)) { RepeatBehavior = RepeatBehavior.Forever });
                mark.Children.Add(arc);
                break;
            case RowState.Done:
                mark.Children.Add(new Ellipse { Fill = B("Accent") });
                mark.Children.Add(Glyph("M 5.5,10.2 L 8.8,13.4 L 14.6,7", B("AccentInk")));
                break;
            case RowState.Failed:
                mark.Children.Add(new Ellipse { Fill = B("Danger") });
                mark.Children.Add(Glyph("M 6.5,6.5 L 13.5,13.5 M 13.5,6.5 L 6.5,13.5", B("Bg")));
                break;
            case RowState.Warn:
                mark.Children.Add(new Ellipse { Fill = B("Warm") });
                mark.Children.Add(Glyph("M 10,5.5 L 10,11 M 10,14.2 L 10,14.4", B("Bg")));
                break;
            case RowState.Info:
                mark.Children.Add(new Ellipse { Stroke = B("Accent"), StrokeThickness = 1.5 });
                mark.Children.Add(Glyph("M 10,9 L 10,14 M 10,6 L 10,6.2", B("Accent")));
                break;
            case RowState.Skipped:
                mark.Children.Add(new Ellipse { Stroke = B("Line2"), StrokeThickness = 1.5 });
                mark.Children.Add(Glyph("M 6.5,10 L 13.5,10", B("FgMute")));
                break;
            default:
                mark.Children.Add(new Ellipse { Stroke = B("Line2"), StrokeThickness = 1.5 });
                break;
        }
    }

    static Path Glyph(string data, Brush brush) => new()
    {
        Data = Geometry.Parse(data), Stroke = brush, StrokeThickness = 2, StrokeStartLineCap = PenLineCap.Round,
        StrokeEndLineCap = PenLineCap.Round, StrokeLineJoin = PenLineJoin.Round,
    };
}

/// <summary>A labelled value with a Copy button (fingerprints, invite codes, addresses).</summary>
public sealed class CopyField : Border
{
    readonly TextBlock label = new() { FontSize = 12, FontWeight = FontWeights.SemiBold };
    readonly TextBox value = new() { IsReadOnly = true, BorderThickness = new Thickness(0), Padding = new Thickness(0), Background = Brushes.Transparent,
        TextWrapping = TextWrapping.Wrap };
    readonly Button copy = new() { Content = "Copy", VerticalAlignment = VerticalAlignment.Center, Margin = new Thickness(12, 0, 0, 0) };

    public CopyField()
    {
        CornerRadius = new CornerRadius(10);
        BorderThickness = new Thickness(1);
        Padding = new Thickness(14, 10, 10, 10);
        Margin = new Thickness(0, 0, 0, 10);
        var dock = new DockPanel();
        DockPanel.SetDock(copy, Dock.Right);
        dock.Children.Add(copy);
        var stack = new StackPanel();
        stack.Children.Add(label);
        stack.Children.Add(value);
        dock.Children.Add(stack);
        Child = dock;
        copy.Click += (_, _) =>
        {
            if (SafeClipboard.TrySetText(Copied ?? value.Text))
            {
                copy.Content = "Copied";
                return;
            }
            // Another program is holding the clipboard: select the text so Ctrl+C works.
            copy.Content = "Try again";
            value.Focus();
            value.SelectAll();
        };
        Loaded += (_, _) =>
        {
            Background = (Brush)FindResource("Bg2");
            BorderBrush = (Brush)FindResource("Line");
            label.Foreground = (Brush)FindResource("FgMute");
            value.Foreground = (Brush)FindResource("Fg");
            value.FontFamily = (FontFamily)FindResource("MonoFont");
            copy.Style = (Style)FindResource("Secondary");
            copy.MinWidth = 72;
            copy.Padding = new Thickness(12, 6, 12, 6);
        };
    }

    public string Label { get => label.Text; set => label.Text = value; }
    public string Value { get => value.Text; set => this.value.Text = value; }
    public double ValueSize { get => value.FontSize; set => this.value.FontSize = value; }
    /// <summary>What Copy puts on the clipboard, when it differs from what's shown.</summary>
    public string? Copied { get; set; }
}
