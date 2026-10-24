using System.Runtime.InteropServices;
using System.Windows;
using System.Windows.Interop;

namespace PPSetup.Ui;

/// <summary>Small Windows touches the WPF API doesn't cover.</summary>
public static class Native
{
    [DllImport("dwmapi.dll")]
    static extern int DwmSetWindowAttribute(IntPtr hwnd, int attr, ref int value, int size);

    const int DarkMode = 20, CaptionColor = 35, TextColor = 36;

    /// <summary>Gives the window a dark title bar in the ParallelPath colours (Windows 11; Windows 10 gets plain dark).</summary>
    public static void DarkTitleBar(Window w)
    {
        var hwnd = new WindowInteropHelper(w).Handle;
        var on = 1;
        DwmSetWindowAttribute(hwnd, DarkMode, ref on, sizeof(int));
        var caption = 0x00201D0F; // #0F1D20 as COLORREF (0x00BBGGRR)
        var text = 0x00F2F3E8;    // #E8F3F2
        DwmSetWindowAttribute(hwnd, CaptionColor, ref caption, sizeof(int));
        DwmSetWindowAttribute(hwnd, TextColor, ref text, sizeof(int));
    }
}
