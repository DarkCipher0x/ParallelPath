using System.IO;
using System.Windows;
using System.Windows.Threading;

namespace PPSetup.Ui;

/// <summary>
/// The setup must never just vanish: an unexpected error is shown, written to a log for support, and the wizard stays
/// open (an install that's halfway through keeps going or reports its own failure).
/// </summary>
public static class Guard
{
    public static string LogPath => Path.Combine(Path.GetTempPath(), "ParallelPath-setup.log");

    public static void Install(Application app, string product)
    {
        app.DispatcherUnhandledException += (_, e) =>
        {
            e.Handled = true;
            Report(app, product, e.Exception);
        };
        TaskScheduler.UnobservedTaskException += (_, e) =>
        {
            e.SetObserved();
            app.Dispatcher.BeginInvoke(() => Report(app, product, e.Exception));
        };
    }

    static void Report(Application app, string product, Exception ex)
    {
        try
        {
            File.AppendAllText(LogPath, $"{DateTime.Now:O} {product}\n{ex}\n\n");
        }
        catch { /* the log is best effort */ }
        if (app.MainWindow is { IsLoaded: true } w)
            Dialog.Error(w, "Something went wrong",
                ex.Message + "\n\nSetup is still open, so you can carry on or try again. Details were saved to " + LogPath + ".");
    }
}
