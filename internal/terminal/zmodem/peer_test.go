package zmodem

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This is an independent raw-wire peer, not a claim of lrzsz verification.
func TestIndependentJSPeer(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	nodePath, err := exec.Command("node", "-p", "process.execPath").Output()
	if err != nil {
		t.Skip("node unavailable")
	}
	for _, direction := range []string{"send", "receive"} {
		t.Run(direction, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			script := `
const Z=require('./node_modules/zmodem.js/src/zsession.js');
const mode=process.argv[1];let session;let init=[];
const output=b=>process.stdout.write(Buffer.from(b));
const payload=Buffer.from(Array.from({length:4096},(_,i)=>i%256));
process.stdin.on('data',b=>{
 try {
 if(!session){init.push(...b);session=Z.Session.parse(init);if(!session)return;
 session.set_sender(output);
 if(mode==='send'){
 session.on('offer',offer=>{const got=[];offer.on('input',chunk=>got.push(...chunk));offer.accept().then(()=>{if(!Buffer.from(got).equals(payload))throw Error('payload differs')})});
 session.on('session_end',()=>process.exit(0));session.start();
 }else{
 session.send_offer({name:'peer.bin',size:payload.length}).then(async x=>{if(!x)throw Error('rejected');await x.end(Array.from(payload));await session.close();process.exit(0)}).catch(e=>{console.error(e);process.exit(2)});
 }
 }else session.consume(Array.from(b));
 }catch(e){console.error(e);process.exit(3)}
});`
			cmd := exec.CommandContext(ctx, strings.TrimSpace(string(nodePath)), "-e", script, direction)
			cmd.Dir = "../../.."
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); _ = cmd.Wait() }()
			write := func(b []byte) error { _, e := input.Write(b); return e }
			payload := make([]byte, 4096)
			for i := range payload {
				payload[i] = byte(i)
			}
			if direction == "send" {
				sender := &Sender{Write: write, Read: func(_ context.Context, b []byte) (int, error) { return output.Read(b) }}
				err = sender.SendFile(ctx, FileMeta{Name: "local.bin"}, payload)
			} else {
				var got []byte
				receiver := &Receiver{Write: write, OnChunk: func(b []byte) error { got = append(got, b...); return nil }}
				err = receiver.Start(ctx)
				if err == nil {
					err = receiver.SessionReader(ctx, output)
				}
				if err == nil && !bytes.Equal(got, payload) {
					t.Fatal("received payload differs")
				}
			}
			if err != nil {
				cancel()
				_ = cmd.Wait()
				t.Fatalf("protocol: %v; peer: %s", err, stderr.String())
			}
			_ = input.Close()
			_, _ = io.Copy(io.Discard, output)
			if err = cmd.Wait(); err != nil {
				t.Fatalf("peer: %v %s", err, stderr.String())
			}
		})
	}
}
