pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Calendar (month) face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Calendar"); gloss: "暦" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "card")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["card", "instrument"], String(opts.g("style", "card"))))
    }
    MenuRow {
        label: I18n.tr("Week starts")
        value: Number(opts.g("weekStart", 1)) === 0 ? I18n.tr("Sunday") : I18n.tr("Monday")
        closeOnTrigger: false
        onTriggered: opts.put("weekStart", Number(opts.g("weekStart", 1)) === 0 ? 1 : 0)
    }
    MenuRow {
        label: I18n.tr("Adjacent days")
        value: opts.g("showAdjacentDays", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showAdjacentDays", true); closeOnTrigger: false
        onTriggered: opts.toggle("showAdjacentDays", true)
    }
    MenuRow {
        label: I18n.tr("Ruled grid")
        value: opts.g("instrumentRule", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("instrumentRule", true); closeOnTrigger: false
        onTriggered: opts.toggle("instrumentRule", true)
    }
}
