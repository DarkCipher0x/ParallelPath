using System.Runtime.InteropServices;
using System.ServiceProcess;
using System.Windows.Threading;
using PPSetup.Ui;
using Drawing = System.Drawing;
using Forms = System.Windows.Forms;

namespace PPSetup.Server.Manager;

/// <summary>
/// The ParallelPath icon by the clock. A small dot shows the server's state (green running, amber starting or
/// stopping, red stopped); clicking it opens the server window, right-clicking gives the menu.
/// </summary>
public sealed class TrayIcon : IDisposable
{
    readonly Forms.NotifyIcon notify = new();
    readonly Forms.ContextMenuStrip menu = new();
    readonly Forms.ToolStripMenuItem header, open, browser, copy, invite, status, start, stop, restart;
    readonly DispatcherTimer timer = new() { Interval = TimeSpan.FromSeconds(4) };
    readonly ServerWindow window = new();

    public ServerWindow Window => window;
    readonly Drawing.Icon baseIcon;
    ServerInfo? info;
    ServiceControllerStatus? state;
    bool firstPoll = true;
    IntPtr shownIcon;

    public TrayIcon(Action exit)
    {
        using (var s = System.Windows.Application.GetResourceStream(new Uri("pack://application:,,,/PPSetup.Ui;component/Assets/parallelpath.ico")).Stream)
            baseIcon = new Drawing.Icon(s, Forms.SystemInformation.SmallIconSize);

        header = Item("ParallelPath Server", null);
        header.Enabled = false;
        open = Item("Open ParallelPath Server", Show);
        var adminItem = Item("Admin portal…", () => { Show(); _ = window.OpenAdmin(); });
        open.Font = new Drawing.Font(open.Font, Drawing.FontStyle.Bold);
        browser = Item("Open in browser", () => { if (info != null) ServerInfo.OpenInBrowser(info.LocalUrl); });
        copy = Item("Copy server address", () => { if (info != null) SafeClipboard.TrySetText(info.FirstAddress); });
        invite = Item("Create an invite code…", () => info?.RunAsAdmin(false, "invite", "--pause"));
        status = Item("Server status…", () => info?.RunAsAdmin(false, "status", "--pause"));
        start = Item("Start server", () => info?.RunAsAdmin(true, "service", "start"));
        stop = Item("Stop server", () => info?.RunAsAdmin(true, "service", "stop"));
        restart = Item("Restart server", () => info?.RunAsAdmin(true, "service", "restart"));
        menu.Items.AddRange([header, new Forms.ToolStripSeparator(), open, adminItem, browser, copy, new Forms.ToolStripSeparator(), invite, status,
            new Forms.ToolStripSeparator(), start, stop, restart, new Forms.ToolStripSeparator(), Item("Exit", exit)]);
        menu.Renderer = new Renderer();
        menu.ShowImageMargin = false;
        menu.Opening += (_, _) => Poll();

        notify.ContextMenuStrip = menu;
        notify.MouseClick += (_, e) => { if (e.Button == Forms.MouseButtons.Left) Show(); };
        notify.BalloonTipClicked += (_, _) => Show();
        timer.Tick += (_, _) => Poll();
        Poll();
        notify.Visible = true;
        timer.Start();
    }

    static Forms.ToolStripMenuItem Item(string text, Action? onClick)
    {
        var i = new Forms.ToolStripMenuItem(text) { Padding = new Forms.Padding(4, 3, 4, 3) };
        if (onClick != null) i.Click += (_, _) => onClick();
        return i;
    }

    public void Show()
    {
        Poll();
        window.Show();
        if (window.WindowState == System.Windows.WindowState.Minimized) window.WindowState = System.Windows.WindowState.Normal;
        window.Activate();
    }

    void Poll()
    {
        info = ServerInfo.Load();
        var now = ServerInfo.State();
        if (!firstPoll && state == ServiceControllerStatus.Running && now == ServiceControllerStatus.Stopped)
            notify.ShowBalloonTip(6000, "ParallelPath Server stopped", "Your family's devices can't connect until it runs again. Click to open.",
                Forms.ToolTipIcon.Warning);
        firstPoll = false;
        if (now != state || shownIcon == IntPtr.Zero) SetIcon(now);
        state = now;
        var running = now == ServiceControllerStatus.Running;
        var label = now switch
        {
            ServiceControllerStatus.Running => "running",
            ServiceControllerStatus.Stopped => "stopped",
            ServiceControllerStatus.StartPending => "starting",
            ServiceControllerStatus.StopPending => "stopping",
            null => "not installed",
            _ => now.ToString()!.ToLowerInvariant(),
        };
        header.Text = "ParallelPath Server - " + label;
        notify.Text = "ParallelPath Server - " + label; // tooltip (63 characters at most)
        browser.Enabled = invite.Enabled = running;
        copy.Enabled = status.Enabled = info != null;
        start.Enabled = info != null && now == ServiceControllerStatus.Stopped;
        stop.Enabled = restart.Enabled = running;
        window.Update(info, now);
    }

    [DllImport("user32.dll")] static extern bool DestroyIcon(IntPtr icon);

    // The ParallelPath icon with a state dot in the corner.
    void SetIcon(ServiceControllerStatus? s)
    {
        var color = s switch
        {
            ServiceControllerStatus.Running => Drawing.Color.FromArgb(0x5F, 0xD3, 0x8D),
            ServiceControllerStatus.Stopped => Drawing.Color.FromArgb(0xFF, 0x7A, 0x7A),
            null => Drawing.Color.FromArgb(0x7A, 0x96, 0x93),
            _ => Drawing.Color.FromArgb(0xF2, 0xA6, 0x5A),
        };
        using var bmp = baseIcon.ToBitmap();
        using (var g = Drawing.Graphics.FromImage(bmp))
        {
            g.SmoothingMode = Drawing.Drawing2D.SmoothingMode.AntiAlias;
            var d = Math.Max(6, bmp.Width * 7 / 16);
            var r = new Drawing.Rectangle(bmp.Width - d, bmp.Height - d, d - 1, d - 1);
            using var fill = new Drawing.SolidBrush(color);
            using var ring = new Drawing.Pen(Drawing.Color.FromArgb(0x0B, 0x15, 0x17), Math.Max(1f, d / 6f));
            g.FillEllipse(fill, r);
            g.DrawEllipse(ring, r);
        }
        var h = bmp.GetHicon();
        notify.Icon = Drawing.Icon.FromHandle(h);
        if (shownIcon != IntPtr.Zero) DestroyIcon(shownIcon);
        shownIcon = h;
    }

    public void Dispose()
    {
        timer.Stop();
        notify.Visible = false;
        notify.Dispose();
        menu.Dispose();
        if (shownIcon != IntPtr.Zero) DestroyIcon(shownIcon);
        window.Exiting = true;
        window.Close();
    }

    // The menu in the ParallelPath colours.
    sealed class Renderer : Forms.ToolStripProfessionalRenderer
    {
        public Renderer() : base(new Colors()) => RoundedEdges = false;

        protected override void OnRenderItemText(Forms.ToolStripItemTextRenderEventArgs e)
        {
            e.TextColor = e.Item.Enabled ? Drawing.Color.FromArgb(0xE8, 0xF3, 0xF2) : Drawing.Color.FromArgb(0x7A, 0x96, 0x93);
            base.OnRenderItemText(e);
        }

        protected override void OnRenderSeparator(Forms.ToolStripSeparatorRenderEventArgs e)
        {
            var y = e.Item.Height / 2;
            using var pen = new Drawing.Pen(Drawing.Color.FromArgb(0x22, 0x39, 0x3D));
            e.Graphics.DrawLine(pen, 6, y, e.Item.Width - 6, y);
        }
    }

    sealed class Colors : Forms.ProfessionalColorTable
    {
        static readonly Drawing.Color Panel = Drawing.Color.FromArgb(0x13, 0x24, 0x27);
        static readonly Drawing.Color Hover = Drawing.Color.FromArgb(0x18, 0x2D, 0x31);
        static readonly Drawing.Color Line = Drawing.Color.FromArgb(0x2E, 0x4B, 0x50);
        static readonly Drawing.Color Accent = Drawing.Color.FromArgb(0x3D, 0xD6, 0xD0);
        public override Drawing.Color ToolStripDropDownBackground => Panel;
        public override Drawing.Color ImageMarginGradientBegin => Panel;
        public override Drawing.Color ImageMarginGradientMiddle => Panel;
        public override Drawing.Color ImageMarginGradientEnd => Panel;
        public override Drawing.Color MenuBorder => Line;
        public override Drawing.Color MenuItemBorder => Accent;
        public override Drawing.Color MenuItemSelected => Hover;
        public override Drawing.Color MenuItemSelectedGradientBegin => Hover;
        public override Drawing.Color MenuItemSelectedGradientEnd => Hover;
        public override Drawing.Color SeparatorDark => Line;
        public override Drawing.Color SeparatorLight => Panel;
    }
}
