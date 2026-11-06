"""Run only inside the disposable systemd container after installation."""
import http.cookiejar, json, pathlib, subprocess, time, urllib.request, urllib.error
state=pathlib.Path('/var/lib/sing-box-manager')
port=json.loads((state/'selected-port.json').read_text())['port']
base=f'http://127.0.0.1:{port}'
client=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
def request(path,body=None,method=None):
    headers={'X-Requested-With':'sing-box-manager','Content-Type':'application/json'}
    req=urllib.request.Request(base+'/api/'+path, data=json.dumps(body).encode() if body is not None else None,headers=headers,method=method)
    try:
        with client.open(req,timeout=60) as r: return r.status,json.load(r)
    except urllib.error.HTTPError as e: return e.code,json.load(e)
assert request('login',{'token':(state/'token').read_text().strip()})[0]==200
p=pathlib.Path('/etc/sing-box/config.json'); original=p.read_bytes()
# Prepare only a draft, then validate/apply the installed binary's native API.
assert request('monitoring/prepare',{})[0]==200
assert p.read_bytes()==original, 'Preparing monitoring must never change the live file'
assert request('validate',{})[0]==200
code,result=request('apply',{}); assert code==200,(code,result)
live=json.loads(p.read_text())
api=next(x for x in live['services'] if x['type']=='api')
assert api['listen']=='127.0.0.1' and api['listen_port']==9095 and api['secret']
print('PASS: native monitoring prepare, validate, safe apply and loopback configuration',flush=True)

def stream_until(path,match,timeout=20):
    with client.open(base+'/api/'+path,timeout=timeout) as response:
        assert response.headers['Content-Type'].startswith('text/event-stream')
        deadline=time.monotonic()+timeout; event='message'; data=[]
        while time.monotonic()<deadline:
            line=response.readline().decode().rstrip('\r\n')
            if line.startswith('event:'): event=line[6:].strip()
            elif line.startswith('data:'): data.append(line[5:].strip())
            elif not line and data:
                payload=json.loads('\n'.join(data))
                if match(event,payload): return payload
                data=[];event='message'
    raise AssertionError('Expected SSE event not received: '+path)

sample=stream_until('telemetry',lambda event,payload:event=='telemetry' and payload.get('available') is True)
assert sample.get('timestamp')
print('PASS: authenticated native telemetry SSE emits available named event',flush=True)
stream_until('logs',lambda event,payload:'sing-box started' in str(payload))
print('PASS: actual journald SSE contains sing-box startup entry',flush=True)
# Cancelled streams must release journalctl followers rather than leak processes.
for _ in range(40):
    followers=subprocess.run(['pgrep','-f','journalctl.*sing-box.*-f'],capture_output=True).stdout.strip()
    if not followers: break
    time.sleep(.25)
assert not followers, 'Closing log SSE left journalctl follower running'
print('PASS: disconnected journal SSE releases journalctl process',flush=True)
code,history=request('history');assert code==200 and history
monitoring_bytes=p.read_bytes()
assert request('restore',{'id':history[0]['id']})[0]==200
assert p.read_bytes()==monitoring_bytes, 'History restore must create a draft only'
code,restored=request('config'); assert code==200 and restored['draft']
assert restored['draft']['config']==json.loads(original)
assert request('validate',{})[0]==200
code,result=request('apply',{});assert code==200,(code,result)
assert json.loads(p.read_bytes())==json.loads(original)
# Safe apply preserves JSON semantics; serialization may normalize indentation.
original=p.read_bytes()
print('PASS: history restores draft then safe apply restores original configuration',flush=True)
code,c=request('config'); assert code==200,c
code,c=request('config'); assert code==200,c
bad={**c['config'],'inbounds':[{'type':'does-not-exist','tag':'invalid'}]}
assert request('draft',{'baseHash':c['hash'],'config':bad},'PUT')[0]==200
assert request('apply',{})[0]>=400
assert p.read_bytes()==original
print('PASS: invalid configuration never replaces live file')
# Occupy a port without changing sing-box; checker succeeds but service startup cannot bind.
code,c=request('config')
bad={**c['config'],'inbounds':[{'type':'mixed','tag':'occupied-test','listen':'127.0.0.1','listen_port':port}]}
assert request('draft',{'baseHash':c['hash'],'config':bad},'PUT')[0]==200
code,result=request('apply',{})
print("Runtime apply result:", code, result, flush=True)
assert code>=400,(code,result)
assert p.read_bytes()==original
assert subprocess.check_output(['systemctl','is-active','sing-box']).strip()==b'active'
print('PASS: runtime startup failure restores configuration and running service', result)
# Independent helper restart recovers an interrupted persistent transaction.
subprocess.run(['systemctl','stop','sing-box-manager-helper'],check=True)
h=pathlib.Path('/var/lib/sing-box-manager-helper')
backup=h/'interrupted-test-backup.json';backup.write_bytes(original);backup.chmod(0o600)
p.write_text('{"inbounds":[{"type":"invalid"}]}')
(h/'transaction.json').write_text(json.dumps({'Path':str(p),'Backup':str(backup),'Mode':384,'UID':0,'GID':0,'Phase':'prepared'}))
subprocess.run(['systemctl','start','sing-box-manager-helper'],check=True)
for _ in range(20):
    if p.read_bytes()==original and not (h/"transaction.json").exists(): break
    time.sleep(.5)
assert p.read_bytes()==original
assert subprocess.check_output(['systemctl','is-active','sing-box']).strip()==b'active'
assert not (h/'transaction.json').exists()
print('PASS: helper restart recovers persistent interrupted deployment')
