import QtQuick
import Quickshell

ShellRoot {
    FloatingWindow {
        id: win

        // Untranslated on purpose: the compositors' window rules match this
        // title, and every Quickshell window shares one app id.
        title: "Welcome to Ryoku"
        readonly property int fitW: win.screen
            ? Math.min(1180, win.screen.width - 24) : 1180
        readonly property int fitH: win.screen
            ? Math.min(760, win.screen.height - 56) : 760
        readonly property bool cramped: win.fitW < 1180 || win.fitH < 760

        minimumSize: Qt.size(Math.min(980, win.fitW), Math.min(640, win.fitH))
        maximumSize: win.cramped
            ? Qt.size(win.fitW, win.fitH)
            : Qt.size(16777215, 16777215)
        onClosed: Qt.quit()

        Welcome {
            anchors.fill: parent
        }
    }
}
