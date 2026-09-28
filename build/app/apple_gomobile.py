import os
import subprocess

from app.build import Builder


class AppleGoMobileBuilder(Builder):
    def before_build(self):
        super().before_build()
        self.clean_lib_dirs(["LibXray.xcframework"])
        self.prepare_gomobile()

    def build(self):
        self.before_build()

        os.chdir(self.lib_dir)
        ret = subprocess.run(
            [
                "gomobile",
                "bind",
                "-target",
                "ios,iossimulator,macos,maccatalyst",
                "-iosversion",
                "15.0",
                # Without this gomobile leaves the macOS deployment target to clang,
                # which defaults to the build machine's OS, so the macOS slice would
                # not load on anything older than the Mac that built it. 13.0 is the
                # oldest macOS that Go 1.27 supports.
                "-macosversion",
                "13.0",
            ]
        )
        if ret.returncode != 0:
            raise Exception("build failed")

        self.after_build()

        self.revert_go_env()
