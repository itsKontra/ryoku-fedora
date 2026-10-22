# Anaconda add-on screen for the Ryoku Fedora ISO: when post/provision queued
# the module signing key's enrollment, it stands between the finished install
# and the reboot and shows what MokManager will ask at the next boot. The text
# is provision's notice, with the password ryoku-host reported; nothing here
# knows the password. Without a queued enrollment Anaconda skips the screen.
from pathlib import Path

from pyanaconda.core.i18n import C_
from pyanaconda.ui.gui.hubs.summary import SummaryHub
from pyanaconda.ui.gui.spokes import StandaloneSpoke

__all__ = ["EnrollmentSpoke"]

# where post/stage leaves provision's handoff in the installer environment
HANDOFF = "/run/ryoku-iso"


def queued_notice(handoff):
    """The enrollment notice when ryoku-host queued an enrollment, else None."""
    try:
        answer = Path(handoff, "enrollment").read_text().splitlines()
        notice = Path(handoff, "enrollment-notice").read_text().strip()
    except OSError:
        return None
    if "enrollment=queued" not in answer or not notice:
        return None
    return notice


class EnrollmentSpoke(StandaloneSpoke):
    builderObjects = ["enrollmentWindow"]
    mainWidgetName = "enrollmentWindow"
    uiFile = "enrollment.glade"
    # after the installation progress screen (priority 0), before the reboot
    postForHub = SummaryHub
    priority = 100

    @staticmethod
    def get_screen_id():
        return "ryoku-enrollment"

    @classmethod
    def should_run(cls, environment, data):
        # Anaconda asks only once the progress screen is left, after provision ran.
        return super().should_run(environment, data) and queued_notice(HANDOFF) is not None

    @property
    def completed(self):
        return False

    def apply(self):
        pass

    def initialize(self):
        super().initialize()
        notice = queued_notice(HANDOFF) or ""
        self.builder.get_object("noticeLabel").set_text("\n\n".join(notice.splitlines()))
        self.window.get_continue_button().set_label(C_("GUI|Progress", "_Reboot System"))
        # As on Anaconda's finished progress screen, the install is done: no Quit.
        # Anaconda calls show_all() on every screen it shows, so hiding alone won't hold.
        quit_button = self.window.get_quit_button()
        quit_button.set_no_show_all(True)
        quit_button.hide()
