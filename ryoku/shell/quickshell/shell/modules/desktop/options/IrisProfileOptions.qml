pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Profile (user card) face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Profile"); gloss: "人物" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "card")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["card", "instrument"], String(opts.g("style", "card"))))
    }
    MenuRow {
        label: I18n.tr("Picture")
        value: opts.g("showAvatar", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showAvatar", true); closeOnTrigger: false
        onTriggered: opts.toggle("showAvatar", true)
    }
    MenuRow {
        label: I18n.tr("Weather")
        value: opts.g("showWeather", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showWeather", true); closeOnTrigger: false
        onTriggered: opts.toggle("showWeather", true)
    }
    MenuRow {
        label: I18n.tr("Computer name")
        value: opts.g("showHostname", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showHostname", true); closeOnTrigger: false
        onTriggered: opts.toggle("showHostname", true)
    }
}
