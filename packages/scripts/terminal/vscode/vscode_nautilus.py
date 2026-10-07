"""Nautilus context-menu integration for Visual Studio Code."""

from subprocess import Popen

import gi

gi.require_version("Nautilus", "4.0")
from gi.repository import GObject, Nautilus  # noqa: E402


VSCODE_COMMAND = "code"


def _local_directory_path(file_info):
    """Return the filesystem path for a local Nautilus directory.

    Args:
        file_info (Nautilus.FileInfo): Nautilus location to inspect.

    Returns:
        str | None: The local directory path, or ``None`` for unsupported
        locations.
    """
    if file_info.get_uri_scheme() != "file" or not file_info.is_directory():
        return None

    location = file_info.get_location()
    if location is None:
        return None
    return location.get_path()


class VSCodeMenuProvider(GObject.GObject, Nautilus.MenuProvider):
    """Provide Visual Studio Code actions for local Nautilus directories."""

    def _open_vscode(self, _menu_item, directory):
        """Open a local directory in a new Visual Studio Code window.

        Args:
            _menu_item (Nautilus.MenuItem): Activated Nautilus menu item.
            directory (str): Local directory passed to Visual Studio Code.

        Returns:
            None: Visual Studio Code launches asynchronously.
        """
        Popen([VSCODE_COMMAND, "--new-window", directory])

    def _create_item(self, *, name, directory):
        """Create an Open in VS Code action for a directory.

        Args:
            name (str): Unique Nautilus action identifier.
            directory (str): Local directory to open in Visual Studio Code.

        Returns:
            Nautilus.MenuItem: Configured folder action.
        """
        label = "Open in VS Code"
        item = Nautilus.MenuItem(name=name, label=label, tip=label)
        item.connect("activate", self._open_vscode, directory)
        return item

    def get_file_items(self, files):
        """Return an action for one selected local directory.

        Args:
            files (list[Nautilus.FileInfo]): Current Nautilus selection.

        Returns:
            list[Nautilus.MenuItem]: One action for a supported selection.
        """
        if len(files) != 1:
            return []

        directory = _local_directory_path(files[0])
        if directory is None:
            return []
        return [self._create_item(name="VSCodeOpen::selected_directory", directory=directory)]

    def get_background_items(self, current_folder):
        """Return an action for the current local directory background.

        Args:
            current_folder (Nautilus.FileInfo): Directory currently displayed.

        Returns:
            list[Nautilus.MenuItem]: One action for a supported directory.
        """
        directory = _local_directory_path(current_folder)
        if directory is None:
            return []
        return [self._create_item(name="VSCodeOpen::background_directory", directory=directory)]
