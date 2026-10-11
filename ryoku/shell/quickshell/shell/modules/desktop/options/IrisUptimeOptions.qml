pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Uptime face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Uptime"); gloss: "稼働" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "row")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["row", "instrument"], String(opts.g("style", "row"))))
    }
    MenuRow {
        label: I18n.tr("Start time")
        value: opts.g("showSince", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showSince", true); closeOnTrigger: false
        onTriggered: opts.toggle("showSince", true)
    }
    MenuRow {
        label: I18n.tr("Breakdown")
        value: opts.g("showBreakdown", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showBreakdown", true); closeOnTrigger: false
        onTriggered: opts.toggle("showBreakdown", true)
    }
}
