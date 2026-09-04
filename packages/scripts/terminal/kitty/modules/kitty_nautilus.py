"""Nautilus context-menu integration for locally installed Kitty."""

from pathlib import Path
from subprocess import Popen

import gi

gi.require_version("Nautilus", "4.0")
from gi.repository import GObject, Nautilus  # noqa: E402


KITTY_EXECUTABLE = str(Path.home() / ".local" / "kitty.app" / "bin" / "kitty")


def _local_directory_path(file_info):
    """Return the filesystem path for a local Nautilus directory.

    Args:
        file_info (Nautilus.FileInfo): Nautilus location to inspect. Only the
            ``file`` URI scheme and directory locations are accepted.

    Returns:
        str | None: Absolute local directory path, or ``None`` when the
        location is remote, is not a directory, or has no filesystem path.

    Raises:
        None: Unsupported locations are represented by ``None``.

    Examples:
        A Nautilus directory with URI ``file:///tmp/project`` returns
        ``/tmp/project``. An ``sftp`` location returns ``None``.
    """
    # A remote URI can expose a path-like component that is not a usable local
    # working directory, so only Nautilus's explicit local-file scheme is used.
    if file_info.get_uri_scheme() != "file" or not file_info.is_directory():
        return None

    location = file_info.get_location()
    if location is None:
        return None
    return location.get_path()


class KittyMenuProvider(GObject.GObject, Nautilus.MenuProvider):
    """Provide independent Kitty actions for local Nautilus directories."""

    def _open_kitty(self, _menu_item, directory):
        """Launch Kitty with a local directory as its working directory.

        Args:
            self (KittyMenuProvider): Provider handling the menu activation.
            _menu_item (Nautilus.MenuItem): Activated menu item. Nautilus
                supplies it, but the launch does not depend on its state.
            directory (str): Existing local directory used as Kitty's process
                working directory.

        Returns:
            None: Kitty is launched asynchronously.

        Raises:
            OSError: If Kitty cannot be executed or the directory is invalid.
        """
        Popen([KITTY_EXECUTABLE], cwd=directory)

    def _create_item(self, *, name, label, directory):
        """Create a Kitty menu item bound to a local directory.

        Args:
            self (KittyMenuProvider): Provider that receives item activation.
            name (str): Unique Nautilus action identifier.
            label (str): Text displayed in the Nautilus context menu.
            directory (str): Local working directory passed to Kitty.

        Returns:
            Nautilus.MenuItem: Configured context-menu item.

        Raises:
            None: Nautilus validates its menu-item construction parameters.

        Examples:
            ``_create_item(name="KittyOpen::folder", label="Open in Kitty",
            directory="/tmp")`` creates an action targeting ``/tmp``.
        """
        item = Nautilus.MenuItem(name=name, label=label, tip=label)
        item.connect("activate", self._open_kitty, directory)
        return item

    def get_file_items(self, files):
        """Return an Open in Kitty item for one selected local directory.

        Args:
            self (KittyMenuProvider): Provider evaluating the selection.
            files (list[Nautilus.FileInfo]): Current Nautilus selection. Exactly
                one local directory is required.

        Returns:
            list[Nautilus.MenuItem]: One Kitty item for a supported selection,
            otherwise an empty list.

        Raises:
            None: Unsupported selections produce no menu items.
        """
        if len(files) != 1:
            return []

        directory = _local_directory_path(files[0])
        if directory is None:
            return []

        return [
            self._create_item(
                name="KittyOpen::selected_directory",
                label="Open in Kitty",
                directory=directory,
            )
        ]

    def get_background_items(self, current_folder):
        """Return an Open Kitty Here item for a local directory background.

        Args:
            self (KittyMenuProvider): Provider evaluating the current folder.
            current_folder (Nautilus.FileInfo): Directory displayed by Nautilus.

        Returns:
            list[Nautilus.MenuItem]: One Kitty item for a local directory,
            otherwise an empty list.

        Raises:
            None: Unsupported locations produce no menu items.
        """
        directory = _local_directory_path(current_folder)
        if directory is None:
            return []

        return [
            self._create_item(
                name="KittyOpen::background_directory",
                label="Open Kitty Here",
                directory=directory,
            )
        ]
