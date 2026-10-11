pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS "Up next" (upcoming events) face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Up next"); gloss: "予定" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "card")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["card", "instrument"], String(opts.g("style", "card"))))
    }
    MenuSlider {
        id: maxEv
        label: I18n.tr("Maximum events")
        from: 1; to: 12; step: 1
        value: Number(opts.g("maxEvents", 5))
        valueText: Math.round(maxEv.value)
        onMoved: (v) => opts.put("maxEvents", Math.round(v))
        onReleased: (v) => opts.put("maxEvents", Math.round(v))
    }
    MenuRow {
        label: I18n.tr("Date")
        value: opts.g("showDate", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showDate", true); closeOnTrigger: false
        onTriggered: opts.toggle("showDate", true)
    }
    MenuRow {
        label: I18n.tr("Time")
        value: opts.g("showTime", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showTime", true); closeOnTrigger: false
        onTriggered: opts.toggle("showTime", true)
    }
    MenuRow {
        label: I18n.tr("Group by day")
        value: opts.g("groupByDay", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("groupByDay", true); closeOnTrigger: false
        onTriggered: opts.toggle("groupByDay", true)
    }
    MenuRow {
        label: I18n.tr("Location")
        value: opts.g("showLocation", false) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showLocation", false); closeOnTrigger: false
        onTriggered: opts.toggle("showLocation", false)
    }
}
