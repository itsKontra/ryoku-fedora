pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Batteries face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Batteries"); gloss: "電池" }
    MenuRow {
        label: I18n.tr("Display")
        value: opts.cap(String(opts.g("displayMode", "ring")))
        closeOnTrigger: false
        onTriggered: opts.put("displayMode", opts.cycle(["ring", "bars", "pill"], String(opts.g("displayMode", "ring"))))
    }
    MenuRow {
        label: I18n.tr("Time remaining")
        value: opts.g("showTime", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showTime", true); closeOnTrigger: false
        onTriggered: opts.toggle("showTime", true)
    }
    MenuRow {
        label: I18n.tr("Charge rate")
        value: opts.g("showRate", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showRate", true); closeOnTrigger: false
        onTriggered: opts.toggle("showRate", true)
    }
    MenuSlider {
        id: ring
        visible: String(opts.g("displayMode", "ring")) === "ring"
        label: I18n.tr("Ring size")
        from: 48; to: 120; step: 1
        value: Number(opts.g("ringSize", 72))
        valueText: Math.round(ring.value)
        onMoved: (v) => opts.put("ringSize", Math.round(v))
        onReleased: (v) => opts.put("ringSize", Math.round(v))
    }
}
