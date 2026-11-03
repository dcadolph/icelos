# dmg.py lays out the release disk image for dmgbuild: Icelos on the left and an Applications
# link on the right, with no background image. mac/release.sh passes the app path as a define.

app = defines.get("app", ".build/Icelos.app")  # noqa: F821 (dmgbuild provides defines)

format = "UDZO"
filesystem = "HFS+"
files = [app]
symlinks = {"Applications": "/Applications"}
icon_locations = {"Icelos.app": (150, 180), "Applications": (450, 180)}
window_rect = ((200, 140), (600, 400))
default_view = "icon-view"
icon_size = 128
text_size = 13
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
include_icon_view_settings = True
