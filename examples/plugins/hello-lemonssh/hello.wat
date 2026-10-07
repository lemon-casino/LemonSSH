;; hello-lemonssh — lemonssh-wasm-abi v1 dispatch demo for the Go WASM host.
;;
;; The host (internal/plugin/wasm) writes a JSON request envelope
;;   {"method":"<name>","payload":...}
;; into this module's linear memory behind lemonssh_alloc and calls
;; lemonssh_dispatch(reqPtr, reqLen). This module answers:
;;   ping                      -> {"ok":true,"result":{"pong":true,"greeting":"<setting>"}}
;;   command.execute           -> {"ok":true,"result":{"command":"hello.ping","handled":true}}
;;                                for its declared hello.ping command
;;   providers.list            -> one terminal.theme provider declaration
;;   provider.invoke           -> {"ok":true,"result":{"colors":{"cursor":"#34d399"}}}
;;   anything else             -> structured not_found envelope
;;
;; Host imports are broker-gated by the Go host: each call returns a
;; structured i32 status (0 ok, -1 permission denied, ...) and never traps.
;; Without the manifest's declared ["runtime","log"] write and
;; ["runtime","settings"] read grants, the log line is dropped and the
;; greeting degrades to "" — the plugin keeps working either way. Providers
;; only surface after the trusted host grants the manifest-declared
;; ["provider","terminal.theme"] read permission (internal/plugin/providers).
(module
  (import "lemonssh" "lemonssh_host_log"
    (func $host_log (param $level i32) (param $ptr i32) (param $len i32) (result i32)))
  (import "lemonssh" "lemonssh_host_setting_get"
    (func $host_setting_get
      (param $keyPtr i32) (param $keyLen i32) (param $bufPtr i32) (param $bufCap i32)
      (result i32)))

  (memory (export "memory") 1 4)
  (global $heap (mut i32) (i32.const 4096))

  ;; Fixed data layout (offsets must stay below the $heap start):
  ;;   0    info log line (23 bytes)
  ;;   32   needle `"method":"ping"` (15 bytes)
  ;;   64   declared setting id com.lemonssh.hello.greeting (27 bytes)
  ;;   128  512-byte setting value buffer
  ;;   672  pong prefix (44 bytes, ends at the value colon)
  ;;   728  empty JSON string `""` (2 bytes, denied-placeholder value)
  ;;   720  pong suffix `}}` (2 bytes)
  ;;   768  [u32 LE 68][unknown-method envelope]
  ;;   1024 needle `"method":"command.execute"` (26 bytes)
  ;;   1088 needle `"command":"hello.ping"` (22 bytes)
  ;;   1152 needle `"method":"providers.list"` (25 bytes)
  ;;   1216 needle `"method":"provider.invoke"` (26 bytes)
  ;;   1280 [u32 LE 60][command.execute ok envelope]
  ;;   1408 [u32 LE 118][providers.list envelope]
  ;;   1664 [u32 LE 52][provider.invoke theme envelope]
  ;;   1792 needle `"method":"provider.sessionEvent"` (30 bytes)
  ;;   1856 [u32 LE 11][session-event ack envelope]
  (data (i32.const 0) "hello-lemonssh dispatch")
  (data (i32.const 32) "\"method\":\"ping\"")
  (data (i32.const 64) "com.lemonssh.hello.greeting")
  (data (i32.const 672) "{\"ok\":true,\"result\":{\"pong\":true,\"greeting\":")
  (data (i32.const 728) "\"\"")
  (data (i32.const 720) "}}")
  (data (i32.const 768) "\44\00\00\00{\"ok\":false,\"error\":{\"code\":\"not_found\",\"message\":\"unknown method\"}}")
  (data (i32.const 1024) "\"method\":\"command.execute\"")
  (data (i32.const 1088) "\"command\":\"hello.ping\"")
  (data (i32.const 1152) "\"method\":\"providers.list\"")
  (data (i32.const 1216) "\"method\":\"provider.invoke\"")
  (data (i32.const 1280) "\3c\00\00\00{\"ok\":true,\"result\":{\"command\":\"hello.ping\",\"handled\":true}}")
  (data (i32.const 1408) "\76\00\00\00{\"ok\":true,\"result\":{\"providers\":[{\"id\":\"com.lemonssh.hello.accent\",\"kind\":\"terminal.theme\",\"label\":\"Hello Accent\"}]}}")
  (data (i32.const 1664) "\34\00\00\00{\"ok\":true,\"result\":{\"colors\":{\"cursor\":\"#34d399\"}}}")
  (data (i32.const 1792) "\"method\":\"provider.sessionEvent\"")
  (data (i32.const 1856) "\0b\00\00\00{\"ok\":true}")

  ;; Returns 1 when the request bytes contain the needle, else 0. Plain
  ;; substring search: good enough for the demo, not a JSON parser.
  (func $contains (param $h i32) (param $hl i32) (param $n i32) (param $nl i32) (result i32)
    (local $i i32) (local $j i32)
    (block $done
      (loop $outer
        (br_if $done
          (i32.gt_u (i32.add (local.get $i) (local.get $nl)) (local.get $hl)))
        (local.set $j (i32.const 0))
        (block $nomatch
          (loop $inner
            (br_if $nomatch (i32.ge_u (local.get $j) (local.get $nl)))
            (br_if $nomatch (i32.ne
              (i32.load8_u (i32.add (local.get $h) (i32.add (local.get $i) (local.get $j))))
              (i32.load8_u (i32.add (local.get $n) (local.get $j)))))
            (local.set $j (i32.add (local.get $j) (i32.const 1)))
            (br $inner)))
        (br_if $done (i32.ge_u (local.get $j) (local.get $nl)))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $outer)))
    (i32.ge_u (local.get $j) (local.get $nl)))

  (func $copy (param $dst i32) (param $src i32) (param $len i32)
    (local $i i32)
    (block $done
      (loop $next
        (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
        (i32.store8
          (i32.add (local.get $dst) (local.get $i))
          (i32.load8_u (i32.add (local.get $src) (local.get $i))))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $next))))

  ;; Reads the plugin's own declared non-secret greeting setting into the
  ;; buffer at 128. Returns the value byte length, or 0 when the host denied
  ;; the read or the value does not fit. Negative statuses compare unsigned
  ;; larger than the buffer capacity and degrade to 0.
  (func $read_greeting (result i32)
    (local $len i32)
    (local.set $len
      (call $host_setting_get
        (i32.const 64) (i32.const 27) (i32.const 128) (i32.const 512)))
    (if (result i32)
      (i32.gt_u (local.get $len) (i32.const 512))
      (then (i32.const 0))
      (else (local.get $len))))

  ;; Grows memory when $need bytes would exceed the current size. Returns 1 on
  ;; success and 0 when memory.grow fails (-1).
  (func $ensure_capacity (param $need i32) (result i32)
    (if (result i32)
      (i32.gt_u (local.get $need) (i32.mul (memory.size) (i32.const 65536)))
      (then
        (i32.ne
          (memory.grow
            (i32.add
              (i32.div_u
                (i32.sub (local.get $need) (i32.mul (memory.size) (i32.const 65536)))
                (i32.const 65536))
              (i32.const 1)))
          (i32.const -1)))
      (else (i32.const 1))))

  ;; lemonssh_alloc: 8-byte aligned bump allocator. Returns 0 on exhaustion.
  (func $alloc (param $size i32) (result i32)
    (local $ptr i32)
    (local.set $ptr
      (i32.and (i32.add (global.get $heap) (i32.const 7)) (i32.const -8)))
    (if
      (call $ensure_capacity (i32.add (local.get $ptr) (local.get $size)))
      (then (global.set $heap (i32.add (local.get $ptr) (local.get $size))))
      (else (local.set $ptr (i32.const 0))))
    (local.get $ptr))

  ;; lemonssh_free: bump allocation cannot release, so this is a no-op. The
  ;; host calls it best-effort with (ptr, len).
  (func $free (param $ptr i32) (param $len i32))

  ;; Builds {"ok":true,"result":{"pong":true,"greeting":"<value>"}} in a
  ;; fresh alloc region behind a [u32 LE length] prefix. The host-marshaled
  ;; value already carries its JSON quotes; a denied/empty read falls back to
  ;; the `""` placeholder at 728 so the envelope stays valid JSON.
  (func $pong (result i32)
    (local $ptr i32) (local $vlen i32) (local $vptr i32)
    (local.set $vlen (call $read_greeting))
    (if (i32.eqz (local.get $vlen))
      (then
        (local.set $vptr (i32.const 728))
        (local.set $vlen (i32.const 2)))
      (else
        (local.set $vptr (i32.const 128))))
    (local.set $ptr
      (call $alloc
        (i32.add
          (i32.add (i32.const 4) (i32.const 44))
          (i32.add (local.get $vlen) (i32.const 2)))))
    (if (i32.eqz (local.get $ptr))
      (then (return (i32.const 0))))
    (i32.store (local.get $ptr)
      (i32.add (i32.add (i32.const 44) (local.get $vlen)) (i32.const 2)))
    (call $copy (i32.add (local.get $ptr) (i32.const 4)) (i32.const 672) (i32.const 44))
    (call $copy
      (i32.add (i32.add (local.get $ptr) (i32.const 4)) (i32.const 44))
      (local.get $vptr)
      (local.get $vlen))
    (call $copy
      (i32.add
        (i32.add (local.get $ptr) (i32.const 4))
        (i32.add (i32.const 44) (local.get $vlen)))
      (i32.const 720)
      (i32.const 2))
    (local.get $ptr))

  ;; lemonssh_dispatch: best-effort info log, then route by request method:
  ;;   command.execute (hello.ping) -> envelope @1280, other commands -> 768
  ;;   providers.list               -> envelope @1408
  ;;   provider.invoke              -> envelope @1664
  ;;   provider.sessionEvent        -> ack envelope @1856
  ;;   ping                         -> pong
  ;;   anything else                -> static not_found envelope @768
  (func $dispatch (param $reqPtr i32) (param $reqLen i32) (result i32)
    (drop (call $host_log (i32.const 1) (i32.const 0) (i32.const 23)))
    (if (result i32)
      (call $contains (local.get $reqPtr) (local.get $reqLen) (i32.const 1024) (i32.const 26))
      (then
        (if (result i32)
          (call $contains (local.get $reqPtr) (local.get $reqLen) (i32.const 1088) (i32.const 22))
          (then (i32.const 1280))
          (else (i32.const 768))))
      (else
        (if (result i32)
          (call $contains (local.get $reqPtr) (local.get $reqLen) (i32.const 1152) (i32.const 25))
          (then (i32.const 1408))
          (else
            (if (result i32)
              (call $contains (local.get $reqPtr) (local.get $reqLen) (i32.const 1216) (i32.const 26))
              (then (i32.const 1664))
              (else
                (if (result i32)
                  (call $contains (local.get $reqPtr) (local.get $reqLen) (i32.const 1792) (i32.const 30))
                  (then (i32.const 1856))
                  (else
                    (if (result i32)
                      (call $contains (local.get $reqPtr) (local.get $reqLen) (i32.const 32) (i32.const 15))
                      (then (call $pong))
                      (else (i32.const 768))))))))))))

  (func $start)

  (export "lemonssh_alloc" (func $alloc))
  (export "lemonssh_free" (func $free))
  (export "lemonssh_dispatch" (func $dispatch))
  (export "_start" (func $start))
)
