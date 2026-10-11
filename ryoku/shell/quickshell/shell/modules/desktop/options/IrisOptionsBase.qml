pragma ComponentBehavior: Bound
import QtQuick
import "../Singletons"
import inir.modules.common as Inir
import inir.services as InirServices
import "../iris/IrisRoster.js" as IrisRoster

// The config plumbing every vendored iRiS face's options panel shares. A hosted
// face resolves each option on its own output first and only then globally
// (DesktopWidgetLayout.value), so the panel reads the same way and writes back
// to whichever of the two the face is actually showing.
Column {
    id: opts

    property string widget: ""
    readonly property var face: IrisRoster.byPrefix(opts.widget)
    readonly property string entry: opts.face ? opts.face.id : ""
    readonly property string path: "background.widgets." + opts.entry
    readonly property string output: Config.writeMonitor

    width: parent ? parent.width : 0
    spacing: Theme.s1

    function g(key, fb) {
        return InirServices.DesktopWidgetLayout.value(opts.output, opts.entry, key, fb);
    }
    function put(key, v) {
        if (opts.output !== "" && InirServices.DesktopWidgetLayout.hasValue(opts.output, opts.entry, key))
            InirServices.DesktopWidgetLayout.setValue(opts.output, opts.entry, key, v);
        else
            Inir.Config.setNestedValue(opts.path + "." + key, v);
    }
    function toggle(key, fb) { opts.put(key, !opts.g(key, fb)); }
    function cap(s) { return (s && s.length > 0) ? s.charAt(0).toUpperCase() + s.slice(1) : s; }
    function cycle(list, cur) { const i = list.indexOf(cur); return list[(i + 1) % list.length]; }
}
