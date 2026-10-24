using System.Runtime.InteropServices;

namespace PPSetup.Ui;

/// <summary>
/// Copying that can't crash. WPF's Clipboard goes through OLE, which on some PCs fails every time with
/// CLIPBRD_E_CANT_OPEN (and throws, which closed the setup). This uses the plain Win32 clipboard instead, retries for
/// a moment while another program (a clipboard manager, clipboard history, Remote Desktop) has it open, and reports
/// failure rather than throwing.
/// </summary>
public static class SafeClipboard
{
    const uint CfUnicodeText = 13, GmemMoveable = 0x0002;

    [DllImport("user32.dll", SetLastError = true)] static extern bool OpenClipboard(IntPtr owner);
    [DllImport("user32.dll")] static extern bool CloseClipboard();
    [DllImport("user32.dll")] static extern bool EmptyClipboard();
    [DllImport("user32.dll")] static extern IntPtr SetClipboardData(uint format, IntPtr data);
    [DllImport("kernel32.dll")] static extern IntPtr GlobalAlloc(uint flags, UIntPtr bytes);
    [DllImport("kernel32.dll")] static extern IntPtr GlobalLock(IntPtr mem);
    [DllImport("kernel32.dll")] static extern bool GlobalUnlock(IntPtr mem);
    [DllImport("kernel32.dll")] static extern IntPtr GlobalFree(IntPtr mem);

    public static bool TrySetText(string text)
    {
        for (var attempt = 0; attempt < 20; attempt++)
        {
            if (OpenClipboard(IntPtr.Zero))
            {
                try
                {
                    return EmptyClipboard() && Put(text);
                }
                finally
                {
                    CloseClipboard();
                }
            }
            Thread.Sleep(50);
        }
        return false;
    }

    static bool Put(string text)
    {
        var bytes = (text.Length + 1) * 2;
        var mem = GlobalAlloc(GmemMoveable, (UIntPtr)bytes);
        if (mem == IntPtr.Zero) return false;
        var p = GlobalLock(mem);
        if (p == IntPtr.Zero)
        {
            GlobalFree(mem);
            return false;
        }
        Marshal.Copy(text.ToCharArray(), 0, p, text.Length);
        Marshal.WriteInt16(p, text.Length * 2, 0);
        GlobalUnlock(mem);
        if (SetClipboardData(CfUnicodeText, mem) != IntPtr.Zero) return true; // the clipboard owns the memory now
        GlobalFree(mem);
        return false;
    }
}
