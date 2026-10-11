pragma ComponentBehavior: Bound
import QtQuick
import Quickshell
import Ryoku.Ui.Singletons
import shell.services as Services
import "../stage/Singletons" as StageCfg

// The system desktop right-click menu: right-clicking bare wallpaper on any bar
// style opens this. It carries a quick row
// of icon tiles (wallpaper thumbnail, search) over the desktop's own actions,
// led by the one way into the Stage Editor -- in Ryoku's paper-and-ink language, on the shared
// DesktopMenu chrome. Every action targets the screen the menu opened on (the
// owning Desktop's slice), never the focused output, which can be a different
// monitor. The per-widget menu (WidgetMenu) and plugin menu are separate.
Item {
    id: menu

    anchors.fill: parent

    // Exposed so the host menu surface maps only while this menu is on screen.
    readonly property alias showing: shell.showing

    // The owning Desktop surface: its screen slice is where the editors and the
    // launcher open, and its wallpaper feeds the Wallpaper tile's thumbnail.
    property var desktop: null

    function openAt(x, y) { shell.px = x; shell.py = y; shell.open = true; }
    function close() { shell.open = false; }

    // The launcher and the Stage Editor open on the monitor the menu is on: the
    // owning desktop's slice, falling back to the focused output only if the menu
    // was built without one.
    function targetState() {
        return menu.desktop ? menu.desktop.stageState : Services.ShellState.forActive();
    }
    function activeMonitor() {
        const st = menu.targetState();
        return (st && st.modelData) ? st.modelData.name : "";
    }

    // The one way into the Stage Editor: widgets, the visualiser, the depth
    // stage and the wallpaper's framing are all its catalogues now, so the
    // menu enters the mode on this monitor and the toolbar does the rest.
    function editDesktop() {
        StageCfg.StageSession.enterWidgets(menu.activeMonitor());
        menu.close();
    }
    function newFolder() {
        const openX = shell.px;
        const openY = shell.py;
        menu.close();
        if (menu.desktop)
            menu.desktop.newDesktopFolder(openX, openY);
    }
    function changeWallpaper() {
        Services.ShellState.requestSurfaceActive("wallpaper", null);
        menu.close();
    }
    // Search opens this monitor's app launcher (the Super+Space surface).
    function openSearch() {
        const st = menu.targetState();
        if (st)
            st.launcherOpen = true;
        menu.close();
    }
    // Quick controls opens the Super+Esc Controls panel, the same surface the
    // `quicksettings` shortcut routes to; Settings opens Ryoku Settings on its
    // normal page, while Edit desktop above owns the Stage Editor entry.
    function quickControls() {
        Services.ShellState.requestSurfaceActive("sidebar-left", undefined);
        menu.close();
    }
    function openSettings() {
        Spawn.run(["ryoku-shell", "hub", "open"]);
        menu.close();
    }
    function refreshShell() {
        Quickshell.execDetached(["ryoku-shell", "reload"]);
        menu.close();
    }

    // The wallpaper thumbnail for the Wallpaper tile: the still the desktop paints
    // (a poster for a video wall), so the tile shows what is on screen.
    readonly property string wallpaperThumb: menu.desktop ? (menu.desktop.wallpaperPath || "") : ""

    DesktopMenu {
        id: shell
        title: I18n.tr("Desktop")
        gloss: "卓上"

        // ── quick row: the wallpaper picker and the launcher ──
        MenuQuick {
            items: [
                { icon: "wallpaper", label: I18n.tr("Wallpaper"), image: menu.wallpaperThumb, action: () => menu.changeWallpaper() },
                { icon: "search", label: I18n.tr("Search"), action: () => menu.openSearch() }
            ]
        }

        MenuSection {}

        MenuRow { icon: "dashboard_customize"; label: I18n.tr("Edit desktop"); accent: true; onTriggered: menu.editDesktop() }
        MenuRow { icon: "create_new_folder"; label: I18n.tr("New folder"); onTriggered: menu.newFolder() }
        MenuRow { icon: "tune"; label: I18n.tr("Quick controls"); onTriggered: menu.quickControls() }

        MenuSection {}
        MenuRow { icon: "settings"; label: I18n.tr("Settings"); accent: true; closeOnTrigger: false; onTriggered: menu.openSettings() }
        MenuRow { icon: "refresh"; label: I18n.tr("Reload shell"); closeOnTrigger: false; onTriggered: menu.refreshShell() }
    }
}
