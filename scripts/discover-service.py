#!/usr/bin/env python3
"""Read-only effective systemd discovery. Never starts, reloads or edits a unit."""
import argparse, os, pathlib, re, subprocess, sys
UNIT = re.compile(r'^[A-Za-z0-9_.@-]+\.service$')
def show(unit):
    result=subprocess.run(['systemctl','show',unit,'--property=Id,LoadState,ActiveState,ExecStart','--no-pager'],text=True,capture_output=True,timeout=10)
    if result.returncode: raise ValueError('Cannot inspect systemd unit '+unit)
    return dict(line.split('=',1) for line in result.stdout.splitlines() if '=' in line)
def direct_path(properties):
    paths=re.findall(r'(?:^|\{\s*)path=([^;]+)\s*;',properties.get('ExecStart',''))
    paths=[p.strip() for p in paths]
    return paths[0] if len(paths)==1 and pathlib.PurePosixPath(paths[0]).name=='sing-box' else None
def validate(unit,properties):
    if properties.get('LoadState')!='loaded': raise ValueError(unit+' is not loaded; repair the existing installation first')
    path=direct_path(properties)
    if not path: raise ValueError(unit+' does not directly execute a sing-box binary; configure an explicit supported service instead of guessing wrapper semantics')
    if not os.path.isfile(path) or not os.access(path,os.X_OK): raise ValueError(unit+' executable is missing or not executable; repair it first')
    if properties.get('ActiveState')=='failed': raise ValueError(unit+' is failed; repair it before installing Manager')
    version=subprocess.run([path,'version'],text=True,capture_output=True,timeout=15)
    if version.returncode or not version.stdout.startswith('sing-box version '): raise ValueError(unit+' binary version check failed; nothing overwritten')
    return unit
def discover(explicit=None):
    if explicit:
        if not UNIT.fullmatch(explicit): raise ValueError('Invalid unit name')
        return validate(explicit,show(explicit))
    default=show('sing-box.service')
    if default.get('LoadState')=='loaded': return validate('sing-box.service',default)
    if default.get('LoadState') not in ('not-found',None): raise ValueError('sing-box.service is masked or broken; repair it first')
    result=subprocess.run(['systemctl','list-unit-files','--type=service','--no-legend','--no-pager'],text=True,capture_output=True,timeout=15,check=True)
    loaded=subprocess.run(['systemctl','list-units','--all','--type=service','--plain','--no-legend','--no-pager'],text=True,capture_output=True,timeout=15,check=True)
    names=set()
    for line in (result.stdout+'\n'+loaded.stdout).splitlines():
        if line.split(): names.add(line.split()[0])
    candidates={}
    wrappers=[]
    for name in sorted(names):
        if not UNIT.fullmatch(name) or '@.' in name: continue
        properties=show(name)
        if direct_path(properties): candidates[properties.get('Id',name)]=(name,properties)
        elif 'sing-box' in properties.get('ExecStart','') and 'sing-box-manager' not in properties.get('ExecStart',''): wrappers.append(name)
    if len(candidates)>1: raise ValueError('Multiple sing-box services detected: '+', '.join(sorted(candidates))+'; select one with --unit before installation')
    if candidates:
        name,properties=next(iter(candidates.values()))
        return validate(name,properties)
    if wrappers: raise ValueError('Possible wrapper-based sing-box services: '+', '.join(wrappers)+'; configure a direct supported unit explicitly; nothing installed')
    return 'sing-box.service'  # Truly absent service; installer separately checks partial files/binary.
def main():
    parser=argparse.ArgumentParser();parser.add_argument('--unit');args=parser.parse_args()
    try: print(discover(args.unit))
    except (ValueError,subprocess.SubprocessError,OSError) as error:
        print('Service discovery refused: '+str(error),file=sys.stderr);sys.exit(1)
if __name__=='__main__': main()
