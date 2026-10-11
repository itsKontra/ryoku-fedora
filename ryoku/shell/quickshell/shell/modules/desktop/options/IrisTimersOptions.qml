pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Timers face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Timers"); gloss: "計時" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "cards")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["cards", "instrument"], String(opts.g("style", "cards"))))
    }
    MenuRow {
        label: I18n.tr("Vertical")
        value: opts.g("vertical", false) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("vertical", false); closeOnTrigger: false
        onTriggered: opts.toggle("vertical", false)
    }
    MenuRow {
        label: I18n.tr("Glow")
        value: opts.g("glow", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("glow", true); closeOnTrigger: false
        onTriggered: opts.toggle("glow", true)
    }
    MenuRow {
        label: I18n.tr("Progress")
        value: opts.g("showProgress", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showProgress", true); closeOnTrigger: false
        onTriggered: opts.toggle("showProgress", true)
    }
    MenuRow {
        label: I18n.tr("State")
        value: opts.g("showState", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showState", true); closeOnTrigger: false
        onTriggered: opts.toggle("showState", true)
    }
    MenuRow {
        label: I18n.tr("Hundredths")
        value: opts.g("showHundredths", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showHundredths", true); closeOnTrigger: false
        onTriggered: opts.toggle("showHundredths", true)
    }
}
