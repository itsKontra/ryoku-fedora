pragma Singleton
import QtQuick
import Ryoku.Ui.Singletons
import Quickshell
import Quickshell.Io

// Live desktop widget config. The legacy top-level values remain the fallback;
// Stage edits add output-keyed overrides in the same widgets.json file.
// FileView watches it so every desktop follows the next write.
Singleton {
    id: root
    property bool ready: false

    // -- clock ---------------------------------------------------------------
    property alias clockEnabled: adapter.clockEnabled
    property alias clockDesign:  adapter.clockDesign   // digital | minimal | analog | flip | rings
    readonly property bool clock24h: TimeFormat.is24h
    property alias clockSeconds: adapter.clockSeconds
    property alias clockAccent:  adapter.clockAccent   // palette | brand | mono
    property alias clockScale:   adapter.clockScale
    property alias clockAnchor:  adapter.clockAnchor   // top-left .. center .. bottom-right | free
    property alias clockX:       adapter.clockX        // free placement, monitor pixels
    property alias clockY:       adapter.clockY
    property alias clockLocked:  adapter.clockLocked   // prevent drag/resize
    property alias clockOpacity: adapter.clockOpacity
    property alias clockBg:      adapter.clockBg        // none | card | glass
    property alias clockRadius:  adapter.clockRadius
    // per-widget ink colour: "" follows the wallpaper (adaptive), a hex pins a
    // solid fill, and Gradient blends <widget>Color -> <widget>Color2 across the
    // rendered glyphs. Only the bare (bg:none) widgets read these.
    property alias clockColor:    adapter.clockColor
    property alias clockColor2:   adapter.clockColor2
    property alias clockGradient: adapter.clockGradient
    property alias dateShow:     adapter.dateShow
    property alias dateDesign:   adapter.dateDesign     // inline | badge | stacked

    // widget typography: the sans family every clock face and Theme.font widget
    // renders with. Empty = the built-in default (Space Grotesk); a bundled or
    // installed family name overrides it. Picked in Hub -> Widgets.
    property alias widgetFont: adapter.widgetFont

    property alias calendarEnabled:       adapter.calendarEnabled
    property alias calendarStyle:         adapter.calendarStyle
    property alias calendarWeeks:         adapter.calendarWeeks
    property alias calendarWeekNumbers:   adapter.calendarWeekNumbers
    property alias calendarHolidayRegion: adapter.calendarHolidayRegion
    property alias calendarScale:         adapter.calendarScale
    property alias calendarAnchor:        adapter.calendarAnchor
    property alias calendarX:             adapter.calendarX
    property alias calendarY:             adapter.calendarY
    property alias calendarLocked:        adapter.calendarLocked
    property alias calendarOpacity:       adapter.calendarOpacity
    property alias calendarColor:    adapter.calendarColor
    property alias calendarColor2:   adapter.calendarColor2
    property alias calendarGradient: adapter.calendarGradient

    property alias musicEnabled: adapter.musicEnabled
    property alias musicStyle:   adapter.musicStyle    // cover | glass
    property alias musicLyrics:  adapter.musicLyrics   // show the synced lyric sheet
    property alias musicViz:     adapter.musicViz     // bars | wave (no-lyrics visualiser look)
    property alias musicScale:   adapter.musicScale
    property alias musicAnchor:  adapter.musicAnchor
    property alias musicX:       adapter.musicX
    property alias musicY:       adapter.musicY
    property alias musicLocked:  adapter.musicLocked
    property alias musicOpacity: adapter.musicOpacity
    property alias musicApp:     adapter.musicApp     // launch command for the corner button
    property alias musicShape:     adapter.musicShape      // wide | tall (9:16)
    property alias musicVideo:     adapter.musicVideo      // off | canvas | custom
    property alias musicVideoFile: adapter.musicVideoFile  // custom backdrop file
    property alias musicColor:    adapter.musicColor
    property alias musicColor2:   adapter.musicColor2
    property alias musicGradient: adapter.musicGradient

    property alias aioEnabled: adapter.aioEnabled
    property alias aioStyle:   adapter.aioStyle     // wide | tall
    property alias aioScale:   adapter.aioScale
    property alias aioAnchor:  adapter.aioAnchor
    property alias aioX:       adapter.aioX
    property alias aioY:       adapter.aioY
    property alias aioLocked:  adapter.aioLocked
    property alias aioOpacity: adapter.aioOpacity
    property alias aioColor:    adapter.aioColor
    property alias aioColor2:   adapter.aioColor2
    property alias aioGradient: adapter.aioGradient

    property alias statsEnabled: adapter.statsEnabled
    property alias statsScale:   adapter.statsScale
    property alias statsAnchor:  adapter.statsAnchor
    property alias statsX:       adapter.statsX
    property alias statsY:       adapter.statsY
    property alias statsLocked:  adapter.statsLocked
    property alias statsOpacity: adapter.statsOpacity
    property alias statsColor:    adapter.statsColor
    property alias statsColor2:   adapter.statsColor2
    property alias statsGradient: adapter.statsGradient

    property alias weatherEnabled: adapter.weatherEnabled
    property alias weatherDesign:  adapter.weatherDesign   // compact | full
    property alias weatherScale:   adapter.weatherScale
    property alias weatherAnchor:  adapter.weatherAnchor
    property alias weatherX:       adapter.weatherX
    property alias weatherY:       adapter.weatherY
    property alias weatherLocked:  adapter.weatherLocked
    property alias weatherOpacity: adapter.weatherOpacity
    property alias weatherColor:    adapter.weatherColor
    property alias weatherColor2:   adapter.weatherColor2
    property alias weatherGradient: adapter.weatherGradient

    property alias notesEnabled: adapter.notesEnabled
    property alias notesScale:   adapter.notesScale
    property alias notesAnchor:  adapter.notesAnchor
    property alias notesX:       adapter.notesX
    property alias notesY:       adapter.notesY
    property alias notesLocked:  adapter.notesLocked
    property alias notesOpacity: adapter.notesOpacity
    property alias notesWidth:   adapter.notesWidth   // pad size in logical px, before scale
    property alias notesHeight:  adapter.notesHeight
    property alias notesColor:    adapter.notesColor
    property alias notesColor2:   adapter.notesColor2
    property alias notesGradient: adapter.notesGradient

    // dayProgress + shape: the two iRiS desktop widgets folded into Ryoku's
    // roster. Both carry the shared must-have contract (enable, placement, size,
    // lock, opacity, colour) plus their own look keys.
    property alias dayprogressEnabled:  adapter.dayprogressEnabled
    property alias dayprogressStyle:    adapter.dayprogressStyle   // ring | arc
    property alias dayprogressShowDate: adapter.dayprogressShowDate
    property alias dayprogressScale:    adapter.dayprogressScale
    property alias dayprogressAnchor:   adapter.dayprogressAnchor
    property alias dayprogressX:        adapter.dayprogressX
    property alias dayprogressY:        adapter.dayprogressY
    property alias dayprogressLocked:   adapter.dayprogressLocked
    property alias dayprogressOpacity:  adapter.dayprogressOpacity
    property alias dayprogressColor:    adapter.dayprogressColor
    property alias dayprogressColor2:   adapter.dayprogressColor2
    property alias dayprogressGradient: adapter.dayprogressGradient

    property alias shapeEnabled:  adapter.shapeEnabled
    property alias shapeKind:     adapter.shapeKind          // dot | ring | diamond | square
    property alias shapeOutline:  adapter.shapeOutline
    property alias shapeScale:    adapter.shapeScale
    property alias shapeAnchor:   adapter.shapeAnchor
    property alias shapeX:        adapter.shapeX
    property alias shapeY:        adapter.shapeY
    property alias shapeLocked:   adapter.shapeLocked
    property alias shapeOpacity:  adapter.shapeOpacity
    property alias shapeColor:    adapter.shapeColor
    property alias shapeColor2:   adapter.shapeColor2
    property alias shapeGradient: adapter.shapeGradient

    // brand: the desktop's mark + name, user-overridable from Ryoku Settings ->
    // Shell -> Global. a small cross-cutting identity master (like theme.json).
    // markText is the glyph/short-text seal (default 力); markImage an optional
    // image path that wins over the text; markTint recolours a single-colour
    // image to the accent; name is the wordmark ("Ryoku") shown in chrome copy.
    // Ryoku's own apps (the Hub, ryo* apps) never read this and keep the 力 brand.
    property alias markText:  brandAdapter.markText
    property alias markImage: brandAdapter.markImage
    property alias markTint:  brandAdapter.markTint
    property alias brandName: brandAdapter.name

    // The desktop sets this before opening one of its menus. Once that output
    // owns a fork, the unchanged menu API writes back to that output instead
    // of changing the legacy values followed by every unforked output.
    property string writeMonitor: ""
    property bool writeMonitorForced: false
    // FileView replaces var-valued adapter properties while a watched atomic
    // write reloads. Keep the live forks detached so that reload cannot erase
    // an edit made after the write it is returning.
    property var _monitorState: ({})
    property int _monitorRevision: 0
    property int _flushedMonitorRevision: 0
    property int _loadingMonitorRevision: 0

    function _plainMonitorMap(value) {
        if (!value || typeof value !== "object")
            return {};
        try {
            return JSON.parse(JSON.stringify(value)) || {};
        } catch (e) {
            return {};
        }
    }

    function _finishLoad() {
        if (root._loadingMonitorRevision === root._monitorRevision) {
            root._monitorState = root._plainMonitorMap(adapter.monitors);
            root._flushedMonitorRevision = root._monitorRevision;
        } else {
            adapter.monitors = root._plainMonitorMap(root._monitorState);
        }
        root.ready = true;
    }

    function selectMonitor(monitor, forced = false) {
        root.writeMonitor = monitor || "";
        root.writeMonitorForced = root.writeMonitor !== "" && forced;
    }

    function _usesMonitorContext() {
        return root.writeMonitor !== ""
            && (root.writeMonitorForced || root.isForked(root.writeMonitor));
    }

    function _monitorMap(monitor) {
        const all = root._monitorState;
        if (!monitor || !all || typeof all !== "object")
            return null;
        const local = all[monitor];
        return local && typeof local === "object" ? local : null;
    }

    function isForked(monitor) {
        return root._monitorMap(monitor) !== null;
    }

    function get(key, monitor) {
        if (!root._isPerMonitorKey(key))
            return adapter[key];
        const local = root._monitorMap(monitor);
        return local && Object.prototype.hasOwnProperty.call(local, key)
            ? local[key] : adapter[key];
    }

    // Find the widget owning a schema key from its <widget>Enabled property.
    // This keeps hosted rosters extensible: adding a new face to the adapter
    // automatically makes every one of that face's keys fork together.
    function _widgetPrefix(key) {
        let found = "";
        const keys = Object.keys(adapter);
        for (let i = 0; i < keys.length; i++) {
            const candidate = keys[i];
            if (!candidate.endsWith("Enabled"))
                continue;
            const prefix = candidate.slice(0, -7);
            if (key.startsWith(prefix) && prefix.length > found.length)
                found = prefix;
        }
        return found;
    }

    function _belongsToWidget(key, prefix) {
        return key.startsWith(prefix);
    }

    function _isPerMonitorKey(key) {
        const prefix = root._widgetPrefix(key);
        if (prefix === "")
            return false;
        const suffix = key.slice(prefix.length);
        return ["Enabled", "Anchor", "X", "Y", "Scale", "Locked",
            "Size", "Width", "Height"].indexOf(suffix) >= 0;
    }

    // Fork one widget from the values this output currently sees. Reassigning
    // the nested map is deliberate: JsonAdapter cannot observe in-place edits.
    function _ensureWidgetFork(all, monitor, key) {
        const current = all[monitor];
        const local = current && typeof current === "object"
            ? Object.assign({}, current) : {};
        const prefix = root._widgetPrefix(key);
        let hasWidget = prefix === "";
        if (prefix !== "") {
            const localKeys = Object.keys(local);
            for (let i = 0; i < localKeys.length; i++) {
                if (root._belongsToWidget(localKeys[i], prefix)
                        && root._isPerMonitorKey(localKeys[i])) {
                    hasWidget = true;
                    break;
                }
            }
        }
        if (!hasWidget) {
            const schemaKeys = Object.keys(adapter);
            for (let i = 0; i < schemaKeys.length; i++) {
                const schemaKey = schemaKeys[i];
                if (schemaKey !== "monitors" && root._belongsToWidget(schemaKey, prefix)
                        && root._isPerMonitorKey(schemaKey)
                        && typeof adapter[schemaKey] !== "function")
                    local[schemaKey] = root.get(schemaKey, monitor);
            }
        }
        all[monitor] = local;
        return local;
    }

    function _setManyFor(monitor, values, persist) {
        if (!monitor) {
            for (const key in values)
                if (adapter[key] !== values[key])
                    adapter[key] = values[key];
            if (persist)
                settle.restart();
            return;
        }
        const all = root._plainMonitorMap(root._monitorState);
        let local = null;
        for (const key in values) {
            local = root._ensureWidgetFork(all, monitor, key);
            local[key] = values[key];
            all[monitor] = local;
        }
        root._monitorState = all;
        root._monitorRevision++;
        adapter.monitors = root._plainMonitorMap(all);
        if (persist)
            settle.restart();
    }

    function setFor(monitor, key, value) {
        const patch = {};
        patch[key] = value;
        root._setManyFor(monitor, patch, true);
    }

    function setManyFor(monitor, values) {
        root._setManyFor(monitor, values, true);
    }

    function setLiveFor(monitor, key, value) {
        const patch = {};
        patch[key] = value;
        root._setManyFor(monitor, patch, false);
    }

    // Appearance stays global. Only presence, placement, size and lock use the
    // selected desktop's fork; legacy callers otherwise keep their old API.
    function set(key, value) {
        if (root._usesMonitorContext() && root._isPerMonitorKey(key)) {
            root.setFor(root.writeMonitor, key, value);
            return;
        }
        adapter[key] = value;
        settle.restart();
    }
    function setMany(values) {
        if (!root._usesMonitorContext()) {
            root._setManyFor("", values, true);
            return;
        }
        const local = {};
        const global = {};
        for (const key in values) {
            if (root._isPerMonitorKey(key))
                local[key] = values[key];
            else
                global[key] = values[key];
        }
        if (Object.keys(local).length > 0)
            root._setManyFor(root.writeMonitor, local, false);
        for (const key in global)
            adapter[key] = global[key];
        settle.restart();
    }
    function setLive(key, value) {
        if (root._usesMonitorContext() && root._isPerMonitorKey(key)) {
            root.setLiveFor(root.writeMonitor, key, value);
            return;
        }
        adapter[key] = value;
    }
    function toggle(key) {
        const monitor = root._usesMonitorContext() && root._isPerMonitorKey(key)
            ? root.writeMonitor : "";
        root.set(key, !root.get(key, monitor));
    }
    function setAnchor(prefix, zone) {
        root.set(prefix + "Anchor", zone);
    }
    function setFree(prefix, x, y) {
        const patch = {};
        patch[prefix + "Anchor"] = "free";
        patch[prefix + "X"] = x;
        patch[prefix + "Y"] = y;
        root.setMany(patch);
    }

    Timer {
        id: settle
        interval: 400
        onTriggered: {
            adapter.monitors = root._plainMonitorMap(root._monitorState);
            root._flushedMonitorRevision = root._monitorRevision;
            file.writeAdapter();
        }
    }

    FileView {
        id: file
        path: (Quickshell.env("XDG_CONFIG_HOME") || (Quickshell.env("HOME") + "/.config")) + "/ryoku/widgets.json"
        blockLoading: true
        watchChanges: true
        printErrors: false
        atomicWrites: true
        onFileChanged: {
            if (settle.running || root._monitorRevision !== root._flushedMonitorRevision)
                return;
            root._loadingMonitorRevision = root._monitorRevision;
            reload();
        }
        onLoaded: root._finishLoad()
        onLoadFailed: root.ready = true

        JsonAdapter {
            id: adapter
            property var monitors: ({})
            property bool clockEnabled: true
            property string clockDesign: "digital"
            // Retired: kept so a write does not drop the pre-global value before
            // ryoku doctor folds it into general.clock_format_24_h.
            property bool clock24h: true
            property bool clockSeconds: false
            property string clockAccent: "palette"
            property real clockScale: 1.0
            property string clockAnchor: "top-left"
            property int clockX: 72
            property int clockY: 64
            property bool clockLocked: false
            property real clockOpacity: 1.0
            property string clockBg: "none"
            property int clockRadius: 26
            property string clockColor: ""
            property string clockColor2: ""
            property bool clockGradient: false
            property bool dateShow: true
            property string dateDesign: "inline"
            property string widgetFont: ""
            property bool calendarEnabled: true
            property string calendarStyle: "glass"
            property int calendarWeeks: 6
            property bool calendarWeekNumbers: true
            property string calendarHolidayRegion: ""
            property real calendarScale: 1.0
            property string calendarAnchor: "bottom-right"
            property int calendarX: 80
            property int calendarY: 80
            property bool calendarLocked: false
            property real calendarOpacity: 1.0
            property string calendarColor: ""
            property string calendarColor2: ""
            property bool calendarGradient: false
            property bool musicEnabled: false
            property string musicStyle: "cover"
            property bool musicLyrics: true
            property string musicViz: "bars"
            property real musicScale: 1.0
            property string musicAnchor: "bottom-left"
            property int musicX: 80
            property int musicY: 80
            property bool musicLocked: false
            property real musicOpacity: 1.0
            property string musicApp: ""
            property string musicShape: "wide"
            property string musicVideo: "canvas"
            property string musicVideoFile: ""
            property string musicColor: ""
            property string musicColor2: ""
            property bool musicGradient: false
            property bool aioEnabled: false
            property string aioStyle: "wide"
            property real aioScale: 1.0
            property string aioAnchor: "top-right"
            property int aioX: 80
            property int aioY: 80
            property bool aioLocked: false
            property real aioOpacity: 1.0
            property string aioColor: ""
            property string aioColor2: ""
            property bool aioGradient: false
            property bool statsEnabled: false
            property real statsScale: 1.0
            property string statsAnchor: "bottom-right"
            property int statsX: 80
            property int statsY: 80
            property bool statsLocked: false
            property real statsOpacity: 1.0
            property string statsColor: ""
            property string statsColor2: ""
            property bool statsGradient: false
            property bool weatherEnabled: false
            property string weatherDesign: "compact"
            property real weatherScale: 1.0
            property string weatherAnchor: "top-right"
            property int weatherX: 80
            property int weatherY: 80
            property bool weatherLocked: false
            property real weatherOpacity: 1.0
            property string weatherColor: ""
            property string weatherColor2: ""
            property bool weatherGradient: false
            property bool notesEnabled: false
            property real notesScale: 1.0
            property string notesAnchor: "right"
            property int notesX: 80
            property int notesY: 80
            property bool notesLocked: false
            property real notesOpacity: 1.0
            property int notesWidth: 260
            property int notesHeight: 180
            property string notesColor: ""
            property string notesColor2: ""
            property bool notesGradient: false
            property bool dayprogressEnabled: false
            property string dayprogressStyle: "ring"
            property bool dayprogressShowDate: true
            property real dayprogressScale: 1.0
            property string dayprogressAnchor: "left"
            property int dayprogressX: 80
            property int dayprogressY: 260
            property bool dayprogressLocked: false
            property real dayprogressOpacity: 1.0
            property string dayprogressColor: ""
            property string dayprogressColor2: ""
            property bool dayprogressGradient: false
            property bool shapeEnabled: false
            property string shapeKind: "dot"
            property bool shapeOutline: false
            property real shapeScale: 1.0
            property string shapeAnchor: "bottom-left"
            property int shapeX: 80
            property int shapeY: 240
            property bool shapeLocked: false
            property real shapeOpacity: 1.0
            property string shapeColor: ""
            property string shapeColor2: ""
            property bool shapeGradient: false
        }
    }

    // brand identity master (mark + name), the cross-cutting identity shared with
    // doctor, the Hub editor and the rest of the shell. the always-on
    // pill seeds it; these defaults cover its absence, so no seed is written here.
    FileView {
        id: brandFile
        path: (Quickshell.env("XDG_CONFIG_HOME") || (Quickshell.env("HOME") + "/.config")) + "/ryoku/brand.json"
        blockLoading: true
        watchChanges: true
        printErrors: false
        atomicWrites: true
        onFileChanged: reload()
        JsonAdapter {
            id: brandAdapter
            property string markText: "力"
            property string markImage: ""
            property bool markTint: true
            property string name: "Ryoku"
        }
    }

    Component.onCompleted: if (!file.text()) file.writeAdapter();
}
