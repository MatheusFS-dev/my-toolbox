import importlib.util
import builtins
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

PACKAGE_ROOT = Path(__file__).resolve().parents[1]

try:
    import tomllib
except ImportError:
    sys.path.insert(0, str(PACKAGE_ROOT / "_vendor"))
    import tomli as tomllib


def load_tool(module_name: str):
    """Load one toolbox script as an importable test module.

    Args:
        module_name (str): Script basename without the Python suffix.

    Returns:
        module: Loaded Python module.

    Raises:
        ImportError: If Python cannot create or execute the module specification.
    """
    path = PACKAGE_ROOT / f"{module_name}.py"
    spec = importlib.util.spec_from_file_location(module_name, path)
    if spec is None or spec.loader is None:
        raise ImportError(f"Could not load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


def load_tool_without_tomllib(module_name: str):
    """Load one toolbox script while making stdlib tomllib unavailable.

    Args:
        module_name (str): Script basename without the Python suffix.

    Returns:
        module: Loaded Python module using its bundled TOML fallback.

    Raises:
        ImportError: If Python cannot create or execute the module specification.
    """
    original_import = builtins.__import__

    def import_without_tomllib(name, globals=None, locals=None, fromlist=(), level=0):
        """Delegate imports except for the deliberately unavailable tomllib.

        Args:
            name (str): Module name requested by the import statement.
            globals (dict | None): Importing module globals.
            locals (dict | None): Importing module locals.
            fromlist (tuple[str, ...]): Requested imported attributes.
            level (int): Relative import level.

        Returns:
            module: Imported module returned by the real import implementation.

        Raises:
            ImportError: When stdlib tomllib is requested or an import fails.
        """
        if name == "tomllib":
            raise ImportError("tomllib deliberately unavailable in fallback test")
        return original_import(name, globals, locals, fromlist, level)

    loaded_name = f"{module_name}_fallback"
    path = PACKAGE_ROOT / f"{module_name}.py"
    spec = importlib.util.spec_from_file_location(loaded_name, path)
    if spec is None or spec.loader is None:
        raise ImportError(f"Could not load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[loaded_name] = module
    with mock.patch("builtins.__import__", side_effect=import_without_tomllib):
        spec.loader.exec_module(module)
    return module


class EnvironmentAliasTest(unittest.TestCase):
    """Verify environment validation and explicit conflict behavior."""

    @classmethod
    def setUpClass(cls) -> None:
        """Load the alias module.

        Args:
            None.

        Returns:
            None.

        Raises:
            ImportError: If the module cannot be loaded.
        """
        cls.tool = load_tool("create_env_alias")

    def test_project_dot_venv_updates_only_selected_shell(self) -> None:
        """Resolve a project venv and write only the selected shell file."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            venv = root / "project" / ".venv"
            (venv / "bin").mkdir(parents=True)
            (venv / "bin" / "activate").write_text("activate\n", encoding="utf-8")
            (venv / "bin" / "python").write_text("python\n", encoding="utf-8")
            (venv / "bin" / "python").chmod(0o755)
            home = root / "home"
            home.mkdir()

            updates, conflicts = self.tool.prepare_alias_updates(
                self.tool.resolve_venv(root / "project"),
                "project-env",
                ["bash"],
                home,
            )
            self.assertEqual(conflicts, [])
            self.tool.apply_alias_updates(updates, create_backups=False)

            bashrc = (home / ".bashrc").read_text(encoding="utf-8")
            self.assertIn("alias project-env=", bashrc)
            self.assertIn(str(venv.resolve() / "bin" / "activate"), bashrc)
            self.assertFalse((home / ".zshrc").exists())

    def test_replacement_and_backup_choices_are_independent(self) -> None:
        """Replace a conflict and create a backup only when separately selected."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            venv = root / "venv"
            (venv / "bin").mkdir(parents=True)
            (venv / "bin" / "activate").write_text("activate\n", encoding="utf-8")
            (venv / "bin" / "python").write_text("python\n", encoding="utf-8")
            (venv / "bin" / "python").chmod(0o755)
            home = root / "home"
            home.mkdir()
            bashrc = home / ".bashrc"
            original = "export KEEP=1\nalias project-env='source /old/bin/activate'\n"
            bashrc.write_text(original, encoding="utf-8")

            updates, conflicts = self.tool.prepare_alias_updates(
                self.tool.resolve_venv(venv),
                "project-env",
                ["bash"],
                home,
            )
            self.assertEqual(conflicts, [bashrc])
            self.assertEqual(bashrc.read_text(encoding="utf-8"), original)
            backups = self.tool.apply_alias_updates(updates, create_backups=True)

            self.assertEqual(len(backups), 1)
            self.assertEqual(backups[0].read_text(encoding="utf-8"), original)
            self.assertIn("export KEEP=1", bashrc.read_text(encoding="utf-8"))
            self.assertIn(str(venv.resolve()), bashrc.read_text(encoding="utf-8"))

    def test_non_executable_python_is_not_a_valid_venv(self) -> None:
        """Reject a venv whose Python path cannot be executed."""
        with tempfile.TemporaryDirectory() as directory:
            venv = Path(directory) / "venv"
            (venv / "bin").mkdir(parents=True)
            (venv / "bin" / "activate").write_text("activate\n", encoding="utf-8")
            (venv / "bin" / "python").write_text("python\n", encoding="utf-8")
            (venv / "bin" / "python").chmod(0o644)
            with self.assertRaisesRegex(ValueError, "executable"):
                self.tool.resolve_venv(venv)

    def test_alias_activates_venv_whose_path_contains_spaces(self) -> None:
        """Quote the activation path inside the stored alias value."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            venv = root / "project with spaces" / ".venv"
            (venv / "bin").mkdir(parents=True)
            (venv / "bin" / "activate").write_text(
                f"export VIRTUAL_ENV={self.tool.shell_single_quote(str(venv))}\n",
                encoding="utf-8",
            )
            (venv / "bin" / "python").write_text("#!/bin/sh\n", encoding="utf-8")
            (venv / "bin" / "python").chmod(0o755)
            home = root / "home"
            home.mkdir()
            updates, _ = self.tool.prepare_alias_updates(
                self.tool.resolve_venv(venv),
                "project-env",
                ["bash"],
                home,
            )
            self.tool.apply_alias_updates(updates, create_backups=False)

            result = subprocess.run(
                [
                    "bash",
                    "--noprofile",
                    "--norc",
                    "-O",
                    "expand_aliases",
                    "-c",
                    'source "$1"; eval project-env; printf "%s" "$VIRTUAL_ENV"',
                    "bash",
                    str(home / ".bashrc"),
                ],
                check=True,
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.stdout, str(venv.resolve()))

    def test_alias_replacement_treats_backslashes_as_literal_text(self) -> None:
        """Replace an alias without interpreting backslashes as regex groups."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            venv = root / r"project\1" / ".venv"
            (venv / "bin").mkdir(parents=True)
            (venv / "bin" / "activate").write_text("activate\n", encoding="utf-8")
            (venv / "bin" / "python").write_text("#!/bin/sh\n", encoding="utf-8")
            (venv / "bin" / "python").chmod(0o755)
            home = root / "home"
            home.mkdir()
            (home / ".bashrc").write_text("alias project-env='old'\n", encoding="utf-8")

            updates, conflicts = self.tool.prepare_alias_updates(
                self.tool.resolve_venv(venv),
                "project-env",
                ["bash"],
                home,
            )

            self.assertEqual(conflicts, [home / ".bashrc"])
            self.assertIn(r"project\1", updates[home / ".bashrc"])

    def test_blank_path_and_symlink_configuration_are_rejected(self) -> None:
        """Require an explicit venv path and refuse to replace a shell symlink."""
        with mock.patch("builtins.input", return_value=""), self.assertRaisesRegex(
            ValueError, "explicit"
        ):
            self.tool.run_interactive()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            venv = root / "venv"
            (venv / "bin").mkdir(parents=True)
            (venv / "bin" / "activate").write_text("activate\n", encoding="utf-8")
            (venv / "bin" / "python").write_text("#!/bin/sh\n", encoding="utf-8")
            (venv / "bin" / "python").chmod(0o755)
            home = root / "home"
            home.mkdir()
            target = root / "shared-bashrc"
            target.write_text("export KEEP=1\n", encoding="utf-8")
            (home / ".bashrc").symlink_to(target)

            with self.assertRaisesRegex(ValueError, "symlink"):
                self.tool.prepare_alias_updates(venv, "project-env", ["bash"], home)
            self.assertEqual(target.read_text(encoding="utf-8"), "export KEEP=1\n")


class PythonBootstrapTest(unittest.TestCase):
    """Verify dependency inference and preflight behavior."""

    @classmethod
    def setUpClass(cls) -> None:
        """Load the bootstrap module.

        Args:
            None.

        Returns:
            None.

        Raises:
            ImportError: If the module cannot be loaded.
        """
        cls.tool = load_tool("bootstrap_python_from_venv")
        try:
            cls.fallback_tool = load_tool_without_tomllib(
                "bootstrap_python_from_venv"
            )
        except RuntimeError as error:
            cls.fallback_tool = None
            cls.fallback_error = error
        else:
            cls.fallback_error = None

    def make_venv(self, root: Path, distributions: dict[str, list[str]]) -> Path:
        """Create a real venv with controlled distribution metadata.

        Args:
            root (Path): Directory that receives the venv.
            distributions (dict[str, list[str]]): Distribution names mapped to
                top-level import names.

        Returns:
            Path: Created venv root.

        Raises:
            subprocess.CalledProcessError: If venv creation fails.
            OSError: If metadata fixtures cannot be written.

        Examples:
            make_venv(root, {"Requests": ["requests"]})
        """
        venv = root / "venv"
        subprocess.run([sys.executable, "-m", "venv", str(venv)], check=True)
        site_packages = next((venv / "lib").glob("python*/site-packages"))
        for index, (name, modules) in enumerate(distributions.items()):
            metadata = site_packages / f"fixture_{index}-1.0.dist-info"
            metadata.mkdir()
            (metadata / "METADATA").write_text(
                f"Metadata-Version: 2.1\nName: {name}\nVersion: 1.0\n",
                encoding="utf-8",
            )
            (metadata / "top_level.txt").write_text(
                "\n".join(modules) + "\n",
                encoding="utf-8",
            )
        return venv

    def test_bundled_tomli_fallback_has_expected_version(self) -> None:
        """Load the bundled parser as Tomli 2.2.1 when tomllib is unavailable."""
        self.assertIsNone(self.fallback_error, str(self.fallback_error))
        self.assertEqual(self.fallback_tool.tomllib.__version__, "2.2.1")

    def test_bundled_tomli_fallback_preserves_valid_project_toml(self) -> None:
        """Preserve unrelated TOML while updating project metadata via fallback."""
        self.assertIsNone(self.fallback_error, str(self.fallback_error))
        text = "[tool.ruff]\nline-length = 100\n"

        updated = self.fallback_tool.update_pyproject(
            text,
            Path("/tmp/project"),
            ["Requests"],
            (3, 9),
            False,
        )

        parsed = self.fallback_tool.tomllib.loads(updated)
        self.assertEqual(parsed["tool"]["ruff"]["line-length"], 100)
        self.assertEqual(parsed["project"]["dependencies"], ["Requests"])
        self.assertEqual(parsed["project"]["requires-python"], ">=3.9,<3.10")

    def test_bundled_tomli_fallback_rejects_malformed_toml_without_writes(
        self,
    ) -> None:
        """Reject malformed project TOML before any generated file is written."""
        self.assertIsNone(self.fallback_error, str(self.fallback_error))
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "project"
            project.mkdir()
            pyproject = project / "pyproject.toml"
            pyproject.write_text("[project\n", encoding="utf-8")
            (project / "app.py").write_text("import requests\n", encoding="utf-8")
            venv = self.make_venv(root, {"Requests": ["requests"]})

            with self.assertRaises(ValueError):
                self.fallback_tool.prepare_bootstrap(project, venv, True, True)

            self.assertEqual(pyproject.read_text(encoding="utf-8"), "[project\n")
            for filename in (
                "requirements.inferred.txt",
                "requirements.txt",
                ".python-version",
            ):
                self.assertFalse((project / filename).exists(), filename)

    def test_generates_all_files_and_preserves_unrelated_toml(self) -> None:
        """Infer Python and notebook imports as unpinned distribution names."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "project"
            project.mkdir()
            (project / "local_module.py").write_text("VALUE = 1\n", encoding="utf-8")
            (project / "app.py").write_text(
                "import os\nimport requests\nimport local_module\n",
                encoding="utf-8",
            )
            (project / "analysis.ipynb").write_text(
                '{"cells":[{"cell_type":"code","source":["import numpy\\n"]}]}',
                encoding="utf-8",
            )
            (project / "pyproject.toml").write_text(
                "[tool.ruff]\nline-length = 100\n",
                encoding="utf-8",
            )
            venv = self.make_venv(root, {"Requests": ["requests"], "NumPy": ["numpy"]})

            plan = self.tool.prepare_bootstrap(project, venv, True, True)
            self.assertEqual(plan.dependencies, ["NumPy", "Requests"])
            self.assertEqual(plan.unresolved, [])
            self.tool.write_bootstrap_files(plan, create_backups=True)

            for name in (
                "requirements.inferred.txt",
                "requirements.txt",
                "pyproject.toml",
                ".python-version",
            ):
                self.assertTrue((project / name).is_file(), name)
            self.assertEqual(
                (project / "requirements.txt").read_text(encoding="utf-8"),
                "NumPy\nRequests\n",
            )
            pyproject = (project / "pyproject.toml").read_text(encoding="utf-8")
            self.assertIn("[tool.ruff]\nline-length = 100", pyproject)
            self.assertIn('"NumPy"', pyproject)
            self.assertIn("[tool.uv]", pyproject)

    def test_malformed_source_and_ambiguous_mapping_write_nothing(self) -> None:
        """Stop before mutation for syntax failures and ambiguous distributions."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "project"
            project.mkdir()
            pyproject = project / "pyproject.toml"
            original = '[project]\nname = "keep"\nversion = "1.0"\n'
            pyproject.write_text(original, encoding="utf-8")
            venv = self.make_venv(root, {"First": ["shared"], "Second": ["shared"]})

            (project / "broken.py").write_text("def broken(:\n", encoding="utf-8")
            with self.assertRaises(SyntaxError):
                self.tool.prepare_bootstrap(project, venv, True, True)
            self.assertEqual(pyproject.read_text(encoding="utf-8"), original)
            self.assertFalse((project / "requirements.txt").exists())

            (project / "broken.py").write_text("import shared\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "ambiguous"):
                self.tool.prepare_bootstrap(project, venv, True, True)
            self.assertEqual(pyproject.read_text(encoding="utf-8"), original)
            self.assertFalse((project / "requirements.txt").exists())

    def test_malformed_toml_writes_nothing(self) -> None:
        """Reject malformed TOML before creating generated files."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "project"
            project.mkdir()
            pyproject = project / "pyproject.toml"
            pyproject.write_text("[project\n", encoding="utf-8")
            (project / "app.py").write_text("import requests\n", encoding="utf-8")
            venv = self.make_venv(root, {"Requests": ["requests"]})

            with self.assertRaises(ValueError):
                self.tool.prepare_bootstrap(project, venv, True, True)
            self.assertEqual(pyproject.read_text(encoding="utf-8"), "[project\n")
            self.assertFalse((project / "requirements.txt").exists())

    def test_toml_key_text_inside_multiline_string_is_preserved(self) -> None:
        """Update actual project keys without rewriting multiline string content."""
        text = (
            "[project]\n"
            'name = "project"\n'
            'version = "1.0"\n'
            'description = """\n'
            "dependencies = [\n"
            "This is documentation, not a TOML key.\n"
            '"""\n'
        )
        updated = self.tool.update_pyproject(
            text,
            Path("/tmp/project"),
            ["Requests"],
            (3, 12),
            False,
        )
        parsed = tomllib.loads(updated)
        self.assertEqual(
            parsed["project"]["description"],
            "dependencies = [\nThis is documentation, not a TOML key.\n",
        )
        self.assertEqual(parsed["project"]["dependencies"], ["Requests"])

    def test_quoted_and_spaced_toml_headers_are_updated(self) -> None:
        """Recognize valid non-canonical spellings of generated TOML tables."""
        text = (
            '["project"]\n'
            'name = "project"\n'
            'version = "1.0"\n\n'
            '[ "tool" . "uv" ]\n'
            'index-url = "https://example.invalid/simple"\n'
        )
        updated = self.tool.update_pyproject(
            text,
            Path("/tmp/project"),
            ["Requests"],
            (3, 12),
            True,
        )
        parsed = tomllib.loads(updated)
        self.assertEqual(parsed["project"]["dependencies"], ["Requests"])
        self.assertIn("environments", parsed["tool"]["uv"])
        self.assertEqual(
            parsed["tool"]["uv"]["index-url"],
            "https://example.invalid/simple",
        )

    def test_generated_output_symlink_stops_before_writing(self) -> None:
        """Reject a generated-output symlink without replacing its target."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "project"
            project.mkdir()
            target = root / "shared.toml"
            target.write_text('[project]\nname = "keep"\n', encoding="utf-8")
            (project / "pyproject.toml").symlink_to(target)
            venv = self.make_venv(root, {})

            with self.assertRaisesRegex(ValueError, "symlink"):
                self.tool.prepare_bootstrap(project, venv, True, True)
            self.assertEqual(
                target.read_text(encoding="utf-8"),
                '[project]\nname = "keep"\n',
            )
            self.assertFalse((project / "requirements.txt").exists())

    def test_generated_output_directory_stops_before_writing(self) -> None:
        """Reject a non-file output conflict before changing another output."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "project"
            project.mkdir()
            pyproject = project / "pyproject.toml"
            original = '[project]\nname = "keep"\n'
            pyproject.write_text(original, encoding="utf-8")
            (project / "requirements.txt").mkdir()
            venv = self.make_venv(root, {})

            with self.assertRaisesRegex(ValueError, "regular file"):
                self.tool.prepare_bootstrap(project, venv, True, True)
            self.assertEqual(pyproject.read_text(encoding="utf-8"), original)
            self.assertFalse((project / "requirements.inferred.txt").exists())


class ProjectTemplateTest(unittest.TestCase):
    """Verify dynamic recursive discovery and conflict preflight."""

    @classmethod
    def setUpClass(cls) -> None:
        """Load the template module.

        Args:
            None.

        Returns:
            None.

        Raises:
            ImportError: If the module cannot be loaded.
        """
        cls.tool = load_tool("create_project_template")

    def test_recursive_copy_includes_special_entries(self) -> None:
        """Copy dotfiles, empty directories, and symlinks dynamically."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "source"
            destination = root / "destination"
            (source / "nested" / "empty").mkdir(parents=True)
            (source / ".config").write_text("config\n", encoding="utf-8")
            (source / "nested" / "file.txt").write_text("source\n", encoding="utf-8")
            os.symlink("nested/file.txt", source / "link")
            destination.mkdir()
            (destination / "destination-only.txt").write_text(
                "keep\n", encoding="utf-8"
            )

            entries = self.tool.discover_template(source)
            self.assertEqual(self.tool.preflight_copy(source, destination, entries), [])
            self.tool.copy_template(source, destination, entries, overwrite=False)

            self.assertTrue((destination / "nested" / "empty").is_dir())
            self.assertEqual(
                (destination / ".config").read_text(encoding="utf-8"), "config\n"
            )
            self.assertEqual(os.readlink(destination / "link"), "nested/file.txt")
            self.assertEqual(
                (destination / "destination-only.txt").read_text(encoding="utf-8"),
                "keep\n",
            )

    def test_type_mismatch_prevents_copy_and_same_type_can_overwrite(self) -> None:
        """Fail preflight on type mismatch, then overwrite listed file conflicts."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "source"
            destination = root / "destination"
            source.mkdir()
            destination.mkdir()
            (source / "same.txt").write_text("source\n", encoding="utf-8")
            (destination / "same.txt").write_text("destination\n", encoding="utf-8")
            (source / "new.txt").write_text("new\n", encoding="utf-8")
            (source / "directory").mkdir()
            (destination / "directory").write_text("wrong\n", encoding="utf-8")
            entries = self.tool.discover_template(source)

            with self.assertRaisesRegex(ValueError, "type mismatch"):
                self.tool.preflight_copy(source, destination, entries)
            self.assertFalse((destination / "new.txt").exists())
            (destination / "directory").unlink()
            (destination / "directory").mkdir()
            self.assertEqual(
                self.tool.preflight_copy(source, destination, entries),
                [Path("same.txt")],
            )
            self.tool.copy_template(source, destination, entries, overwrite=True)
            self.assertEqual(
                (destination / "same.txt").read_text(encoding="utf-8"), "source\n"
            )
            self.assertEqual(
                (destination / "new.txt").read_text(encoding="utf-8"), "new\n"
            )

    def test_overlap_is_rejected(self) -> None:
        """Reject nested source and destination paths before copying."""
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "source"
            nested = source / "nested"
            nested.mkdir(parents=True)
            entries = self.tool.discover_template(source)
            with self.assertRaisesRegex(ValueError, "overlap"):
                self.tool.preflight_copy(source, nested, entries)
