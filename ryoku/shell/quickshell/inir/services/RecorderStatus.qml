pragma Singleton

import QtQuick
import Quickshell
import shell.services as Ryoku

// Recording bridge. Ryoku's recorder (shell.services Recorder, driving
// ryoku-cmd-screenrecord) owns the capture, its audio options and its clock;
// Shima's island, control grid and palette read and drive it here, so a
// recording started from any surface shows the same state everywhere.
Singleton {
    id: root

    readonly property bool available: true
    readonly property bool isRecording: Ryoku.Recorder.active
    readonly property int elapsedSeconds: Ryoku.Recorder.elapsedSec
    readonly property string effectiveAudioMode: Ryoku.Recorder.optDesktopAudio
        ? (Ryoku.Recorder.optMic ? "both" : "system")
        : (Ryoku.Recorder.optMic ? "microphone" : "none")

    function start(): void { Ryoku.Recorder.start(Ryoku.Recorder.recordArgs()); }
    function stop(): void { Ryoku.Recorder.stop(); }
    function toggle(): void {
        if (root.isRecording)
            root.stop();
        else
            root.start();
    }
    // The recorder's state is live, so there is nothing to poll.
    function scheduleQuickCheck(): void {}
}
