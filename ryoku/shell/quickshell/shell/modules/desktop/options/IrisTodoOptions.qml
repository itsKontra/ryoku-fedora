pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Tasks face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Tasks"); gloss: "課題" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "card")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["card", "instrument"], String(opts.g("style", "card"))))
    }
    MenuRow {
        label: I18n.tr("Ruled lines")
        value: opts.g("instrumentRules", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("instrumentRules", true); closeOnTrigger: false
        onTriggered: opts.toggle("instrumentRules", true)
    }
    MenuRow {
        label: I18n.tr("Show completed")
        value: opts.g("showCompleted", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showCompleted", true); closeOnTrigger: false
        onTriggered: opts.toggle("showCompleted", true)
    }
}
