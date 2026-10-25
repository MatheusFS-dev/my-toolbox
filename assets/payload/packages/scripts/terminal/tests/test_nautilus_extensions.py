"""Behavior tests for the Nautilus folder-action extensions."""

import importlib.util
import sys
import types
import unittest
from pathlib import Path
from unittest.mock import patch


TERMINAL_DIRECTORY = Path(__file__).resolve().parents[1]
KITTY_EXTENSION = TERMINAL_DIRECTORY / "kitty" / "modules" / "kitty_nautilus.py"
VSCODE_EXTENSION = TERMINAL_DIRECTORY / "vscode" / "vscode_nautilus.py"


class FakeGObject:
    """Minimal stand-in for the GObject base class."""


class FakeMenuProvider:
    """Minimal stand-in for Nautilus's menu-provider interface."""


class FakeMenuItem:
    """Store a menu item's label and activation callback."""

    def __init__(self, *, name, label, tip):
        self.name = name
        self.label = label
        self.tip = tip
        self._callback = None
        self._arguments = ()

    def connect(self, signal, callback, *arguments):
        if signal != "activate":
            raise AssertionError(f"unexpected signal: {signal}")
        self._callback = callback
        self._arguments = arguments

    def activate(self):
        self._callback(self, *self._arguments)


class FakeLocation:
    """Represent a local Nautilus location."""

    def __init__(self, path):
        self.path = path

    def get_path(self):
        return self.path


class FakeFileInfo:
    """Represent a local directory selection."""

    def __init__(self, path):
        self.path = path

    def get_uri_scheme(self):
        return "file"

    def is_directory(self):
        return True

    def get_location(self):
        return FakeLocation(self.path)


def load_extension(path, module_name):
    """Load an extension with a small in-process Nautilus substitute."""
    gi_module = types.ModuleType("gi")
    gi_module.require_version = lambda *_args: None
    repository = types.ModuleType("gi.repository")
    repository.GObject = types.SimpleNamespace(GObject=FakeGObject)
    repository.Nautilus = types.SimpleNamespace(
        MenuItem=FakeMenuItem,
        MenuProvider=FakeMenuProvider,
        FileInfo=FakeFileInfo,
    )
    previous_modules = {name: sys.modules.get(name) for name in ("gi", "gi.repository")}
    sys.modules["gi"] = gi_module
    sys.modules["gi.repository"] = repository
    try:
        specification = importlib.util.spec_from_file_location(module_name, path)
        module = importlib.util.module_from_spec(specification)
        specification.loader.exec_module(module)
        return module
    finally:
        for name, previous in previous_modules.items():
            if previous is None:
                del sys.modules[name]
            else:
                sys.modules[name] = previous


class NautilusExtensionTests(unittest.TestCase):
    """Validate the public actions provided by each extension."""

    def test_kitty_uses_the_same_open_in_label_for_folder_and_background(self):
        module = load_extension(KITTY_EXTENSION, "kitty_nautilus_test")
        provider = module.KittyMenuProvider()
        folder = FakeFileInfo("/tmp/project")

        self.assertEqual(provider.get_file_items([folder])[0].label, "Open in Kitty")
        self.assertEqual(provider.get_background_items(folder)[0].label, "Open in Kitty")

    def test_vscode_opens_selected_and_background_folders_in_a_new_window(self):
        self.assertTrue(
            VSCODE_EXTENSION.is_file(),
            "VS Code Nautilus extension is missing",
        )
        module = load_extension(VSCODE_EXTENSION, "vscode_nautilus_test")
        provider = module.VSCodeMenuProvider()
        folder = FakeFileInfo("/tmp/project")

        selected_item = provider.get_file_items([folder])[0]
        background_item = provider.get_background_items(folder)[0]
        self.assertEqual(selected_item.label, "Open in VS Code")
        self.assertEqual(background_item.label, "Open in VS Code")

        with patch.object(module, "Popen") as process:
            selected_item.activate()

        process.assert_called_once_with(["code", "--new-window", "/tmp/project"])
