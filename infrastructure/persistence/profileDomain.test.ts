import assert from "node:assert/strict";
import { test } from "node:test";

import { profileDomainForKey } from "./profileDomain";

test("profileDomainForKey routes vault keys away from settings", () => {
  assert.equal(profileDomainForKey("netcatty_hosts_v1"), "vault");
  assert.equal(profileDomainForKey("netcatty_keys_v1"), "vault");
  assert.equal(profileDomainForKey("netcatty_theme_v1"), "settings");
  assert.equal(profileDomainForKey("netcatty_session_restore_v1"), "sessions");
  assert.equal(profileDomainForKey("netcatty_port_forwarding_v1"), "vault");
  assert.equal(profileDomainForKey("netcatty_sftp_transfer_center_v1"), "vault");
  assert.equal(profileDomainForKey("netcatty_sftp_global_bookmarks_v1"), "vault");
});
