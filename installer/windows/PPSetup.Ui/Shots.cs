using System.IO;
using System.Windows;
using System.Windows.Media;
using System.Windows.Media.Imaging;
using System.Windows.Threading;

namespace PPSetup.Ui;

/// <summary>Renders windows to PNG, for the documentation screenshots and the automated sandbox runs.</summary>
public static class Shots
{
    /// <summary>Lets layout, bindings, Loaded handlers and animations catch up.</summary>
    public static async Task Settle(int ms = 250)
    {
        await Dispatcher.Yield(DispatcherPriority.ApplicationIdle);
        await Task.Delay(ms);
        await Dispatcher.Yield(DispatcherPriority.ApplicationIdle);
    }

    public static void Save(Window w, string path)
    {
        var e = (FrameworkElement)w.Content;
        e.UpdateLayout();
        var dpi = VisualTreeHelper.GetDpi(e);
        var bmp = new RenderTargetBitmap((int)Math.Ceiling(e.ActualWidth * dpi.DpiScaleX), (int)Math.Ceiling(e.ActualHeight * dpi.DpiScaleY),
            dpi.PixelsPerInchX, dpi.PixelsPerInchY, PixelFormats.Pbgra32);
        var bg = new DrawingVisual();
        using (var dc = bg.RenderOpen())
            dc.DrawRectangle(w.Background, null, new Rect(0, 0, e.ActualWidth, e.ActualHeight));
        bmp.Render(bg);
        bmp.Render(e);
        var enc = new PngBitmapEncoder();
        enc.Frames.Add(BitmapFrame.Create(bmp));
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        using var f = File.Create(path);
        enc.Save(f);
    }

    /// <summary>Shows a window where nobody sees it (screenshots of every page, without a flash on screen).</summary>
    public static void ShowOffscreen(Window w)
    {
        w.WindowStartupLocation = WindowStartupLocation.Manual;
        w.Left = -32000;
        w.Top = -32000;
        w.ShowInTaskbar = false;
        w.ShowActivated = false;
        w.Show();
    }
}
