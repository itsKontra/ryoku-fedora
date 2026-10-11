pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Vitals (system monitor) face: the readout
// style and which metrics the face graphs.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Vitals"); gloss: "計測" }
    MenuRow {
        label: I18n.tr("Display")
        value: opts.cap(String(opts.g("displayMode", "bars")))
        closeOnTrigger: false
        onTriggered: opts.put("displayMode", opts.cycle(["bars", "graph", "rings", "tiles"], String(opts.g("displayMode", "bars"))))
    }
    MenuSlider {
        id: bars
        visible: String(opts.g("displayMode", "bars")) === "bars"
        label: I18n.tr("Bars")
        from: 8; to: 64; step: 1
        value: Number(opts.g("barCount", 32))
        valueText: Math.round(bars.value)
        onMoved: (v) => opts.put("barCount", Math.round(v))
        onReleased: (v) => opts.put("barCount", Math.round(v))
    }
    MenuRow {
        label: I18n.tr("Labels")
        value: opts.g("showLabels", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showLabels", true); closeOnTrigger: false
        onTriggered: opts.toggle("showLabels", true)
    }

    MenuSection { label: I18n.tr("Metrics"); gloss: "指標" }
    MenuRow {
        label: I18n.tr("CPU")
        value: opts.g("showCpu", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showCpu", true); closeOnTrigger: false
        onTriggered: opts.toggle("showCpu", true)
    }
    MenuRow {
        label: I18n.tr("Memory")
        value: opts.g("showMemory", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showMemory", true); closeOnTrigger: false
        onTriggered: opts.toggle("showMemory", true)
    }
    MenuRow {
        label: I18n.tr("GPU")
        value: opts.g("showGpu", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showGpu", true); closeOnTrigger: false
        onTriggered: opts.toggle("showGpu", true)
    }
    MenuRow {
        label: I18n.tr("CPU heat")
        value: opts.g("showTemp", false) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showTemp", false); closeOnTrigger: false
        onTriggered: opts.toggle("showTemp", false)
    }
    MenuRow {
        label: I18n.tr("GPU heat")
        value: opts.g("showGpuTemp", false) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showGpuTemp", false); closeOnTrigger: false
        onTriggered: opts.toggle("showGpuTemp", false)
    }
    MenuRow {
        label: I18n.tr("Disk")
        value: opts.g("showDisk", false) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showDisk", false); closeOnTrigger: false
        onTriggered: opts.toggle("showDisk", false)
    }
}
