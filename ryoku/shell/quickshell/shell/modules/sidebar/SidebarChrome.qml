pragma ComponentBehavior: Bound

import QtQuick
import Ryoku.Ui.Singletons
import shell.services

Item {
    id: root

    required property var screen
    required property real s
    required property bool active
    signal closeRequested()

    readonly property string page: SidebarState.activeTab(screen)
    readonly property bool detail: page !== "controls"
    readonly property real pad: Tokens.s4 * s
    readonly property real fittedHeight: pad * 2 + header.height + Tokens.s3 * s
        + (board.item ? board.item.implicitHeight : Tokens.cellH * 4 * s)

    function home(): void {
        SidebarState.selectTab(root.screen, "controls");
    }

    Item {
        id: header
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: root.pad
        height: (Tokens.ctlH + Tokens.s1) * root.s

        CornerButton {
            id: back
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
            visible: root.detail
            s: root.s
            glyph: "arrow_back"
            subtle: true
            Accessible.name: I18n.tr("Back to controls")
            onClicked: root.home()
        }

        Text {
            anchors.left: back.visible ? back.right : parent.left
            anchors.right: headerActions.left
            anchors.leftMargin: back.visible ? Tokens.s1 * root.s : 0
            anchors.rightMargin: Tokens.s2 * root.s
            anchors.verticalCenter: parent.verticalCenter
            text: root.page === "wifi" ? I18n.tr("Wi-Fi")
                : root.page === "bluetooth" ? I18n.tr("Bluetooth")
                : I18n.tr("Controls")
            color: Tokens.ink
            font.family: Tokens.ui
            font.pixelSize: Tokens.fBody * root.s
            font.weight: Font.DemiBold
            elide: Text.ElideRight
        }

        Row {
            id: headerActions
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            spacing: Tokens.s1 * root.s

            CornerButton {
                s: root.s
                glyph: "tune"
                subtle: true
                Accessible.name: I18n.tr("Ryoku Hub")
                onClicked: {
                    root.closeRequested();
                    Spawn.run(["ryoku-shell", "hub", "open", "controls"]);
                }
            }
            CornerButton {
                s: root.s
                glyph: "close"
                subtle: true
                Accessible.name: I18n.tr("Close")
                onClicked: root.closeRequested()
            }
        }
    }

    Loader {
        id: board
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: header.bottom
        anchors.bottom: parent.bottom
        anchors.leftMargin: root.pad
        anchors.rightMargin: root.pad
        anchors.topMargin: Tokens.s3 * root.s
        anchors.bottomMargin: root.pad
        sourceComponent: controls
    }

    Component {
        id: controls
        ControlsBoard {
            s: root.s
            screen: root.screen
            active: root.active
            page: root.page
            onRequestClose: root.closeRequested()
        }
    }
}
