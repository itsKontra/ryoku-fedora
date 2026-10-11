pragma Singleton

import QtQuick
import Quickshell
import Quickshell.Io
import Ryoku.Ui.Singletons
import shell.services
import "lib/dock.js" as DockList

// Shared dock model: canonical desktop-entry pins, running windows and actions
// for the universal dock host and Sumi's in-rail RailDock. The list
// maths live in the tested lib/dock.js; settings persist in shell.json's one
// top-level dock object.
Singleton {
    id: root

    // Icon resolution is not reactive on its own: iconFor() reads the desktop DB and
    // the icon theme through plain function calls, so a binding that resolves before
    // the desktop DB is scanned or the icon-theme cache is warm -- a fresh boot, or
    // right after an app updates its .desktop/icon -- sticks on the generic
    // application-x-executable fallback until a shell reload (the "zen icon keeps
    // resetting to a gear" break). Bump iconRev to force every icon binding to
    // re-resolve: on a desktop-DB change and via a bounded warm-up poll after load.
    property int iconRev: 0
    property int _iconTries: 0

    // A desktop-DB change -- an app installed, updated, or removed -- can add or fix
    // the icon a pin resolves to; re-resolve every dock icon when it fires so a pin
    // recovers its real icon live instead of waiting for a reload.
    Connections {
        target: DesktopEntries
        function onApplicationsChanged() {
            root._canonical = {};
            root.iconRev++;
        }
    }
    // The icon index lands a moment after startup (a `ryoku-shell icons` run);
    // bump iconRev the instant it does so pins re-resolve off the fallback
    // without waiting on the warm-up poll below.
    Connections {
        target: Icons
        function onRevChanged() { root.iconRev++; }
    }
    // The icon-theme cache warms a little after the shell starts and nothing signals
    // it, so an icon can be unresolvable at first paint and findable a moment later.
    // Re-resolve a bounded handful of times over the first few seconds after load, so
    // a pin that came up on the generic fallback heals itself without a reload.
    Timer {
        id: iconWarmup
        interval: 400
        repeat: true
        running: true
        onTriggered: {
            root.iconRev++;
            if (++root._iconTries >= 12)
                iconWarmup.stop();
        }
    }

    // Quickshell applications and a surprising number of desktop files expose
    // more than one identity. Persist desktop-entry ids, then resolve window
    // classes through the desktop database so every dock design groups the same
    // application and aliases do not create duplicate pins.
    readonly property var quickshellApps: ({ "Ryoku Settings": "ryoku-hub", "Ryostore": "ryostore", "ryovm": "ryovm" })
    function plainId(value) {
        return String(value ?? "").trim().replace(/\.desktop$/i, "");
    }
    // The desktop-database lookup is the expensive part of every dock binding,
    // and the same few ids are asked for on each window change; remember each
    // answer until the database itself changes.
    property var _canonical: ({})
    function canonicalId(value) {
        const id = root.plainId(value);
        if (id.length === 0)
            return "";
        const known = root._canonical[id];
        if (known !== undefined)
            return known;
        const entry = DesktopEntries.heuristicLookup(id);
        const canonical = entry?.id ? root.plainId(entry.id) : id.toLowerCase();
        root._canonical[id] = canonical;
        return canonical;
    }
    function sameApp(a, b) {
        const left = root.canonicalId(a);
        return left.length > 0 && left === root.canonicalId(b);
    }
    function normalizedPins(values) {
        const out = [];
        for (const value of Array.from(values ?? [])) {
            const id = root.canonicalId(value?.desktop_id ?? value?.id ?? value);
            if (id.length > 0 && !out.includes(id))
                out.push(id);
        }
        return out;
    }
    function classOf(w) {
        if (!w) return "";
        const cls = w.appId;
        if (cls === "org.quickshell" && root.quickshellApps[w.title])
            return root.canonicalId(root.quickshellApps[w.title]);
        return root.canonicalId(cls);
    }

    // Running windows as { className, address }, id-sorted for a stable order.
    readonly property var clients: {
        const result = [];
        const wins = Wm.windows;
        for (let i = 0; i < wins.length; ++i) {
            const className = root.classOf(wins[i]);
            if (className)
                result.push({ className: className, address: wins[i].id });
        }
        result.sort((a, b) => a.address < b.address ? -1 : (a.address > b.address ? 1 : 0));
        return result;
    }

    // The focused window, or null on a bare desktop.
    readonly property var focusedClient: Wm.focusedWindow

    // True while some window holds focus; a bare desktop reads false, which the
    // dock surface uses to show itself when there is nothing to get out of.
    readonly property bool anyFocused: root.focusedClient !== null

    readonly property string activeClass: root.classOf(root.focusedClient)

    // Pinned first, then running-unpinned in id order.
    function resolve(pinned, activeClients) {
        const canonicalPins = root.normalizedPins(pinned);
        const source = activeClients === undefined ? root.clients : activeClients;
        const canonicalClients = Array.from(source ?? []).map(client => ({
            className: root.canonicalId(client.className),
            address: client.address
        })).filter(client => client.className.length > 0);
        return DockList.resolve(canonicalPins, canonicalClients);
    }
    function pin(pinned, className) {
        const pins = root.normalizedPins(pinned);
        const id = root.canonicalId(className);
        return id.length > 0 && !pins.includes(id) ? pins.concat([id]) : pins;
    }
    function unpin(pinned, className) {
        const id = root.canonicalId(className);
        return root.normalizedPins(pinned).filter(pinId => pinId !== id);
    }

    function countFor(className) {
        const list = root.clients;
        let n = 0;
        for (let i = 0; i < list.length; ++i)
            if (root.sameApp(list[i].className, className)) ++n;
        return n;
    }

    // Fallback pins so an empty dock is not mistaken for a missing one.
    function starterPins() {
        const out = [];
        for (const className of ["kitty", "chromium", "nautilus"])
            if (DesktopEntries.heuristicLookup(className)) out.push(className);
        return out;
    }

    // ── the dock store (shell.json top-level `dock`) ─────────────────────────
    // The dock is a first-class shell surface now, so its look and pins live in
    // one top-level store rather than per bar style. Every consumer reads and
    // writes it through here, so the copy-on-write below is the single path that
    // persists it.

    // The look registry, shaped like Theme.workspaceStyleOptions so one control
    // renders them all. Persisted under `dock.style` (default islands, so an
    // existing desktop is unchanged). A style only changes what the band draws.
    readonly property var styleOptions: [
        { key: "islands", label: I18n.tr("Islands"), detail: I18n.tr("Split pills") },
        { key: "rail",    label: I18n.tr("Rail"),    detail: I18n.tr("One continuous plate") },
        { key: "ledger",  label: I18n.tr("Ledger"),  detail: I18n.tr("Numbered cells") },
        { key: "tanzaku", label: I18n.tr("Tanzaku"), detail: I18n.tr("Hanging strips") },
        { key: "seal",    label: I18n.tr("Seal"),    detail: I18n.tr("Colour means running") }
    ]
    // The designs this desktop carries: Ryoku's islands, or no dock surface. A
    // design name it does not know (a config shared from a fuller install) draws
    // the Ryoku dock rather than nothing.
    readonly property string design: String(root.cfg("design", "ryoku")) === "none" ? "none" : "ryoku"
    onDesignChanged: root.closeMenu()

    function cfg(key, fallback) {
        const d = Config.dock;
        return (d && d[key] !== undefined && d[key] !== null) ? d[key] : fallback;
    }
    function setCfg(key, value) {
        const change = {};
        change[key] = value;
        root.patchCfg(change);
    }
    function patchCfg(change) {
        const cur = Config.dock || {};
        const next = {};
        for (const k in cur) next[k] = cur[k];
        for (const k in change) next[k] = change[k];
        // A fresh object so the live look changes this frame...
        Config.dock = next;
        // ...and a settings.patch so it survives: the shell's shell.json FileView
        // is read-only (no onAdapterUpdated), because the daemon owns that file and
        // serialises every writer through its settings store. Same channel Bar
        // Studio and the qsbar control centre write on.
        cfgCtl.queued += "call settings.patch " + JSON.stringify({ path: "dock", value: next }) + "\n";
        if (cfgCtl.connected)
            cfgCtl.flushQueued();
        else
            cfgCtl.connected = true;
    }
    // customPins marks a list the user chose, so emptying it leaves an empty
    // dock instead of bringing the starter set back.
    function setPinned(array) { root.patchCfg({ pinned: root.normalizedPins(array), customPins: true }); }
    // The stored pin state as-is, for undo: the starter fallback is not a
    // choice to write back.
    function pinState() {
        return { pinned: Array.from(root.cfg("pinned", [])), customPins: root.cfg("customPins", false) === true };
    }
    function restorePinState(state) { root.patchCfg({ pinned: state.pinned, customPins: state.customPins }); }
    function resetPins() { root.restorePinState({ pinned: [], customPins: false }); }
    function movePinned(from, to) {
        const pins = root.pinnedOrStarter();
        if (from < 0 || from >= pins.length || to < 0 || to >= pins.length || from === to)
            return;
        const value = pins.splice(from, 1)[0];
        pins.splice(to, 0, value);
        root.setPinned(pins);
    }
    function togglePin(className) {
        const pins = root.pinnedOrStarter();
        root.setPinned(pins.includes(root.canonicalId(className))
            ? root.unpin(pins, className) : root.pin(pins, className));
    }
    function resolvedEdge() {
        const edge = String(root.cfg("edge", "auto"));
        if (["top", "bottom", "left", "right"].includes(edge))
            return edge;
        if (Config.barStyle === "qsbar")
            return Config.qsbar?.barPosition === "bottom" ? "top" : "bottom";
        return "bottom";
    }
    // A bar style that draws its own dock keeps its options in a sub-object
    // of the one dock store (dock.<design>.<key>).
    function designCfg(designName, key, fallback) {
        const d = root.cfg(designName, {});
        return d && d[key] !== undefined && d[key] !== null ? d[key] : fallback;
    }
    // The daemon's control socket, connected only when there is something to say.
    Socket {
        id: cfgCtl
        path: (Quickshell.env("XDG_RUNTIME_DIR") || "/tmp") + "/ryoku-shell.sock"
        property string queued: ""
        function flushQueued() {
            if (cfgCtl.queued.length === 0)
                return;
            cfgCtl.write(cfgCtl.queued);
            cfgCtl.flush();
            cfgCtl.queued = "";
        }
        onConnectionStateChanged: if (cfgCtl.connected) cfgCtl.flushQueued()
    }

    // The effective pin list: the user's order, or the starter set until the
    // user has chosen pins, so an unconfigured dock still shows something
    // instead of reading as broken. Pins still in a retired store (qsbar's
    // dockPinned, Sumi's frameBars.dock.pinned) count as chosen until
    // `ryoku doctor` merges them in.
    function pinnedOrStarter() {
        const p = root.normalizedPins(root.cfg("pinned", []));
        if (p.length || root.cfg("customPins", false) === true)
            return p;
        const retired = root.normalizedPins(Array.from(Config.qsbar?.dockPinned ?? [])
            .concat(Array.from(Config.frameBars?.dock?.pinned ?? [])));
        return retired.length ? retired : root.normalizedPins(root.starterPins());
    }

    // Desktop-entry icon, then class-as-icon-name; "" so callers can fall back.
    function iconFor(className) {
        void root.iconRev;
        const id = root.canonicalId(className);
        const desktop = DesktopEntries.heuristicLookup(id);
        const byEntry = (desktop && desktop.icon) ? Icons.path(desktop.icon, true) : "";
        return byEntry !== "" ? byEntry : Icons.path(id.toLowerCase(), true);
    }

    // No clients -> launch; focused already -> cycle by address; else focus,
    // preferring a client on the active workspace.
    function activate(className) {
        const id = root.canonicalId(className);
        const wins = Wm.windows;
        const matches = [];
        for (let i = 0; i < wins.length; ++i)
            if (root.sameApp(root.classOf(wins[i]), id))
                matches.push(wins[i]);
        if (matches.length === 0) {
            const entry = DesktopEntries.heuristicLookup(id);
            if (entry)
                AppLaunch.run(entry, null);
            return;
        }
        matches.sort((a, b) => a.id < b.id ? -1 : (a.id > b.id ? 1 : 0));
        const focused = Wm.focusedWindow;
        const active = focused ? focused.id : "";
        const idx = matches.findIndex(m => m.id === active);
        let target;
        if (idx >= 0)
            target = matches[(idx + 1) % matches.length];
        else {
            const wsId = Wm.focusedWorkspace ? Wm.focusedWorkspace.id : "";
            target = matches.find(m => m.workspace === wsId) || matches[0];
        }
        Wm.focusWindow(target.id);
    }

    // Close every window of a class (dock menu Close).
    function closeAll(className) {
        const wins = Wm.windows;
        for (let i = 0; i < wins.length; ++i)
            if (root.sameApp(root.classOf(wins[i]), className))
                Wm.closeWindow(wins[i].id);
    }

    // ── right-click context menu ───────────────────────────────────────────────
    // One menu at a time, owned by the monitor the click came from (menuScreen);
    // the per-monitor DockMenuOverlay renders it and dismisses on an outside click.
    property string menuClass: ""
    property bool menuPinned: false
    property int menuCount: 0
    property real menuGx: 0
    property real menuGy: 0
    property string menuScreen: ""
    property string menuEdge: "bottom"
    property real menuEdgeClear: 54
    readonly property bool menuOpen: root.menuClass !== ""
    function openMenu(className, pinned, count, gx, gy, screenName, edge, edgeClear) {
        root.menuClass = className;
        root.menuPinned = pinned;
        root.menuCount = count;
        root.menuGx = gx;
        root.menuGy = gy;
        root.menuScreen = screenName;
        root.menuEdge = edge;
        root.menuEdgeClear = edgeClear;
    }
    function closeMenu() { root.menuClass = ""; }
    function menuActOpen() {
        if (root.menuCount > 0) {
            const e = DesktopEntries.heuristicLookup(root.menuClass);
            if (e) AppLaunch.run(e, null);
        } else {
            root.activate(root.menuClass);
        }
        root.closeMenu();
    }
    function menuActPin() {
        const pins = root.pinnedOrStarter();
        root.setPinned(root.menuPinned ? root.unpin(pins, root.menuClass)
                                       : root.pin(pins, root.menuClass));
        root.closeMenu();
    }
    function menuActClose() {
        root.closeAll(root.menuClass);
        root.closeMenu();
    }
}
