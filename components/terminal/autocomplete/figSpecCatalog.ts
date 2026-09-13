/**
 * Curated Fig command-spec catalog (bundle-size trim, 2026-09-12).
 *
 * The full @withfig/autocomplete dynamic registry is ~1484 specs and added
 * ~75 MiB of per-command chunks to the Wails bundle, dominated by cloud
 * vendor CLIs (aws/az/gcloud subcommand trees). This catalog lazy-imports
 * ONLY curated daily + dev/ops commands via direct relative file imports
 * (the package exports map blocks deep bare-specifier imports). Cloud
 * vendor CLIs are intentionally excluded; re-add entries here when needed.
 *
 * Every entry was verified to exist in the package build directory; spec
 * generators from the registry are never executed.
 */
import type { FigSpec } from './figSpecLoader';

type SpecLoader = () => Promise<{ default: unknown }>;

const load_git: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/git.js');
const load_docker: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/docker.js');
const load_kubectl: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/kubectl.js');
const load_npm: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/npm.js');
const load_yarn: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/yarn.js');
const load_pnpm: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/pnpm.js');
const load_ls: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/ls.js');
const load_cd: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/cd.js');
const load_cat: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/cat.js');
const load_grep: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/grep.js');
const load_find: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/find.js');
const load_ssh: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/ssh.js');
const load_scp: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/scp.js');
const load_curl: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/curl.js');
const load_wget: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/wget.js');
const load_tar: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/tar.js');
const load_zip: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/zip.js');
const load_unzip: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/unzip.js');
const load_make: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/make.js');
const load_python: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/python.js');
const load_python3: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/python3.js');
const load_pip: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/pip.js');
const load_pip3: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/pip3.js');
const load_node: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/node.js');
const load_systemctl: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/systemctl.js');
const load_apt: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/apt.js');
const load_brew: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/brew.js');
const load_vim: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/vim.js');
const load_nano: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/nano.js');
const load_less: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/less.js');
const load_head: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/head.js');
const load_tail: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/tail.js');
const load_sort: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/sort.js');
const load_sed: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/sed.js');
const load_chmod: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/chmod.js');
const load_chown: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/chown.js');
const load_cp: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/cp.js');
const load_mv: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/mv.js');
const load_rm: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/rm.js');
const load_mkdir: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/mkdir.js');
const load_docker_compose: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/docker-compose.js');
const load_helm: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/helm.js');
const load_helmfile: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/helmfile.js');
const load_k9s: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/k9s.js');
const load_kind: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/kind.js');
const load_minikube: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/minikube.js');
const load_podman: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/podman.js');
const load_terraform: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/terraform.js');
const load_ansible: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/ansible.js');
const load_ansible_playbook: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/ansible-playbook.js');
const load_htop: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/htop.js');
const load_top: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/top.js');
const load_ps: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/ps.js');
const load_kill: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/kill.js');
const load_killall: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/killall.js');
const load_df: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/df.js');
const load_du: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/du.js');
const load_ln: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/ln.js');
const load_crontab: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/crontab.js');
const load_rsync: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/rsync.js');
const load_sftp: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/sftp.js');
const load_dig: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/dig.js');
const load_traceroute: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/traceroute.js');
const load_jq: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/jq.js');
const load_bat: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/bat.js');
const load_exa: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/exa.js');
const load_eza: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/eza.js');
const load_rg: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/rg.js');
const load_fd: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/fd.js');
const load_fzf: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/fzf.js');
const load_tmux: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/tmux.js');
const load_screen: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/screen.js');
const load_xargs: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/xargs.js');
const load_tee: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/tee.js');
const load_http: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/http.js');
const load_mysql: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/mysql.js');
const load_psql: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/psql.js');
const load_sqlite3: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/sqlite3.js');
const load_mongosh: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/mongosh.js');
const load_pg_dump: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/pg_dump.js');
const load_deno: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/deno.js');
const load_bun: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/bun.js');
const load_dart: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/dart.js');
const load_flutter: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/flutter.js');
const load_java: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/java.js');
const load_gem: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/gem.js');
const load_bundle: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/bundle.js');
const load_composer: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/composer.js');
const load_dotnet: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/dotnet.js');
const load_go: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/go.js');
const load_gcc: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/gcc.js');
const load_clang: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/clang.js');
const load_gplusplus: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/g++.js');
const load_cmake: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/cmake.js');
const load_gradle: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/gradle.js');
const load_mvn: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/mvn.js');
const load_bazel: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/bazel.js');
const load_poetry: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/poetry.js');
const load_conda: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/conda.js');
const load_pytest: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/pytest.js');
const load_cargo: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/cargo.js');
const load_rustc: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/rustc.js');
const load_next: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/next.js');
const load_nuxt: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/nuxt.js');
const load_vue: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/vue.js');
const load_ng: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/ng.js');
const load_astro: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/astro.js');
const load_vite: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/vite.js');
const load_turbo: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/turbo.js');
const load_prisma: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/prisma.js');
const load_eslint: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/eslint.js');
const load_prettier: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/prettier.js');
const load_jest: SpecLoader = () => import('../../../node_modules/@withfig/autocomplete/build/jest.js');

// Repo-owned local overrides (shared with the Electron baseline under
// electron/specs/); vendored here so the Wails bundle serves them too.
const load_awk: SpecLoader = () => import('./localSpecs/awk.js');
const load_dnf: SpecLoader = () => import('./localSpecs/dnf.js');
const load_journalctl: SpecLoader = () => import('./localSpecs/journalctl.js');
const load_yum: SpecLoader = () => import('./localSpecs/yum.js');

export const CURATED_FIG_SPEC_LOADERS: Readonly<Record<string, SpecLoader>> = Object.freeze({
  'awk': load_awk,
  'dnf': load_dnf,
  'journalctl': load_journalctl,
  'yum': load_yum,
  'git': load_git,
  'docker': load_docker,
  'kubectl': load_kubectl,
  'npm': load_npm,
  'yarn': load_yarn,
  'pnpm': load_pnpm,
  'ls': load_ls,
  'cd': load_cd,
  'cat': load_cat,
  'grep': load_grep,
  'find': load_find,
  'ssh': load_ssh,
  'scp': load_scp,
  'curl': load_curl,
  'wget': load_wget,
  'tar': load_tar,
  'zip': load_zip,
  'unzip': load_unzip,
  'make': load_make,
  'python': load_python,
  'python3': load_python3,
  'pip': load_pip,
  'pip3': load_pip3,
  'node': load_node,
  'systemctl': load_systemctl,
  'apt': load_apt,
  'brew': load_brew,
  'vim': load_vim,
  'nano': load_nano,
  'less': load_less,
  'head': load_head,
  'tail': load_tail,
  'sort': load_sort,
  'sed': load_sed,
  'chmod': load_chmod,
  'chown': load_chown,
  'cp': load_cp,
  'mv': load_mv,
  'rm': load_rm,
  'mkdir': load_mkdir,
  'docker-compose': load_docker_compose,
  'helm': load_helm,
  'helmfile': load_helmfile,
  'k9s': load_k9s,
  'kind': load_kind,
  'minikube': load_minikube,
  'podman': load_podman,
  'terraform': load_terraform,
  'ansible': load_ansible,
  'ansible-playbook': load_ansible_playbook,
  'htop': load_htop,
  'top': load_top,
  'ps': load_ps,
  'kill': load_kill,
  'killall': load_killall,
  'df': load_df,
  'du': load_du,
  'ln': load_ln,
  'crontab': load_crontab,
  'rsync': load_rsync,
  'sftp': load_sftp,
  'dig': load_dig,
  'traceroute': load_traceroute,
  'jq': load_jq,
  'bat': load_bat,
  'exa': load_exa,
  'eza': load_eza,
  'rg': load_rg,
  'fd': load_fd,
  'fzf': load_fzf,
  'tmux': load_tmux,
  'screen': load_screen,
  'xargs': load_xargs,
  'tee': load_tee,
  'http': load_http,
  'mysql': load_mysql,
  'psql': load_psql,
  'sqlite3': load_sqlite3,
  'mongosh': load_mongosh,
  'pg_dump': load_pg_dump,
  'deno': load_deno,
  'bun': load_bun,
  'dart': load_dart,
  'flutter': load_flutter,
  'java': load_java,
  'gem': load_gem,
  'bundle': load_bundle,
  'composer': load_composer,
  'dotnet': load_dotnet,
  'go': load_go,
  'gcc': load_gcc,
  'clang': load_clang,
  'g++': load_gplusplus,
  'cmake': load_cmake,
  'gradle': load_gradle,
  'mvn': load_mvn,
  'bazel': load_bazel,
  'poetry': load_poetry,
  'conda': load_conda,
  'pytest': load_pytest,
  'cargo': load_cargo,
  'rustc': load_rustc,
  'next': load_next,
  'nuxt': load_nuxt,
  'vue': load_vue,
  'ng': load_ng,
  'astro': load_astro,
  'vite': load_vite,
  'turbo': load_turbo,
  'prisma': load_prisma,
  'eslint': load_eslint,
  'prettier': load_prettier,
  'jest': load_jest,
});

export const CURATED_FIG_SPEC_NAMES: readonly string[] = Object.keys(CURATED_FIG_SPEC_LOADERS);
