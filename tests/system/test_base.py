from machinebeat import BaseTest

import os


class Test(BaseTest):

    def test_base(self):
        """
        Basic test with exiting Machinebeat normally
        """
        self.render_config_template(
            path=os.path.abspath(self.working_dir) + "/log/*"
        )

        machinebeat_proc = self.start_beat()
        self.wait_until(lambda: self.log_contains("machinebeat is running"))
        exit_code = machinebeat_proc.kill_and_wait()
        assert exit_code == 0
