import assert from "node:assert/strict";
import { test } from "node:test";

import { profileDomainForKey } from "./profileDomain";

test("profileDomainForKey routes vault keys away from settings", () => {
  assert.equal(profileDomainForKey("lemonssh_hosts_v1"), "vault");
  assert.equal(profileDomainForKey("lemonssh_keys_v1"), "vault");
  assert.equal(profileDomainForKey("lemonssh_theme_v1"), "settings");
  assert.equal(profileDomainForKey("lemonssh_session_restore_v1"), "sessions");
  assert.equal(profileDomainForKey("lemonssh_port_forwarding_v1"), "vault");
  assert.equal(profileDomainForKey("lemonssh_sftp_transfer_center_v1"), "vault");
  assert.equal(profileDomainForKey("lemonssh_sftp_global_bookmarks_v1"), "vault");
});
