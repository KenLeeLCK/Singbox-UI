"""Isolated download/extraction/Tailscale safety tests. No host services modified."""
import importlib.util, io, os, pathlib, subprocess, tarfile, tempfile, unittest
ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('release', ROOT / 'scripts/release.py')
release = importlib.util.module_from_spec(spec); spec.loader.exec_module(release)
class ReleaseTests(unittest.TestCase):
    def test_checksums(self):
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d)/'download'; p.write_bytes(b'abc')
            release.verify(p,'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
            with self.assertRaises(ValueError): release.verify(p,'0'*64)
            with self.assertRaises(ValueError): release.verify(p,'bad')
    def test_archive_escape(self):
        for name,kind in [('../escape',tarfile.REGTYPE),('/absolute',tarfile.REGTYPE),('link',tarfile.SYMTYPE)]:
            with tempfile.TemporaryDirectory() as d:
                p=pathlib.Path(d)/'a.tar.gz'
                with tarfile.open(p,'w:gz') as a:
                    i=tarfile.TarInfo(name); i.type=kind; i.linkname='/etc/passwd'; a.addfile(i)
                with self.assertRaises(ValueError): release.extract(p,pathlib.Path(d)/'out')
    def test_valid_archive(self):
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d)/'a.tar.gz'
            with tarfile.open(p,'w:gz') as a:
                i=tarfile.TarInfo('manager'); i.size=3; a.addfile(i,io.BytesIO(b'abc'))
            release.extract(p,pathlib.Path(d)/'out')
            self.assertEqual((pathlib.Path(d)/'out/manager').read_bytes(),b'abc')
    def test_shell_syntax(self):
        for p in (ROOT/'scripts').glob('*.sh'): subprocess.run(['bash','-n',str(p)],check=True)
    def test_tailscale_existing_listener_preserved(self):
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d)/'tailscale'
            p.write_text('#!/bin/sh\ncase "$*" in\n"status --json") echo \'{"BackendState":"Running"}\';;\n"serve status --json") echo \'{"TCP":{"443":{"HTTPS":true}}}\';;\n*) exit 99;;\nesac\n'); p.chmod(0o755)
            result=subprocess.run(['bash',str(ROOT/'scripts/tailscale-setup.sh'),'5031'],env={**os.environ,'PATH':d+':'+os.environ['PATH']},capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stderr); self.assertIn('preserved unchanged',result.stdout)
    def test_pinned_release_rejects_mutable_version(self):
        with tempfile.TemporaryDirectory() as d:
            p=subprocess.run(['python3',str(ROOT/'scripts/release.py'),'manager','--arch','amd64','--directory',d,'--repo','owner/repo','--version','latest'],capture_output=True)
            self.assertNotEqual(p.returncode,0)
class DiscoveryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec=importlib.util.spec_from_file_location('discovery',ROOT/'scripts/discover-service.py')
        cls.discovery=importlib.util.module_from_spec(spec);spec.loader.exec_module(cls.discovery)
    def test_direct_executable_detection(self):
        self.assertEqual(self.discovery.direct_path({'ExecStart':'{ path=/opt/custom/sing-box ; argv[]=/opt/custom/sing-box run ; }'}),'/opt/custom/sing-box')
        self.assertIsNone(self.discovery.direct_path({'ExecStart':'{ path=/bin/sh ; argv[]=/bin/sh -c sing-box run ; }'}))
        self.assertIsNone(self.discovery.direct_path({'ExecStart':'{ path=/opt/sing-box ; }; { path=/opt/sing-box ; }'}))
    def test_loaded_default_has_priority(self):
        from unittest.mock import patch
        with patch.object(self.discovery,'show',return_value={'LoadState':'loaded'}), patch.object(self.discovery,'validate',return_value='sing-box.service') as validate:
            self.assertEqual(self.discovery.discover(),'sing-box.service')
            validate.assert_called_once()
    def test_broken_service_is_never_absence(self):
        with self.assertRaisesRegex(ValueError,'missing or not executable'):
            self.discovery.validate('custom.service',{'LoadState':'loaded','ExecStart':'{ path=/certainly/missing/sing-box ; }'})
if __name__=='__main__': unittest.main()
