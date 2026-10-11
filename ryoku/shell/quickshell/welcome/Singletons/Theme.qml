pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io

// Welcome-local values only; the look is Ryoku.Ui's Tokens. What remains is the
// brand identity this once-per-login window reads for itself.
Singleton {
    // brand mark + name, user-overridable via ~/.config/ryoku/brand.json (Shell ->
    // Global). defaults to the 力 seal / "Ryoku". BrandMark renders `mark`, or
    // `markSource` (an image) when set.
    readonly property string mark: brandAdapter.markText.length > 0 ? brandAdapter.markText : "力"
    readonly property string markSource: brandAdapter.markImage
    readonly property bool markTint: brandAdapter.markTint
    readonly property string brandName: brandAdapter.name.length > 0 ? brandAdapter.name : "Ryoku"

    // read-only here: the always-on pill seeds the file, so this window falls
    // back to the adapter defaults when it is absent.
    FileView {
        path: (Quickshell.env("XDG_CONFIG_HOME") || (Quickshell.env("HOME") + "/.config")) + "/ryoku/brand.json"
        blockLoading: true
        watchChanges: true
        printErrors: false
        onFileChanged: reload()
        JsonAdapter {
            id: brandAdapter
            property string markText: "力"
            property string markImage: ""
            property bool markTint: true
            property string name: "Ryoku"
        }
    }
}
