type LocalOptions = Parameters<NonNullable<NetcattyBridge['startLocalSession']>>[0];
export interface NativeLocalShellBindings {
 StartLocalWithOptions?: (request: {shell:string;shellArgs:string[];cwd:string;cols:number;rows:number;env:Record<string,string>}) => Promise<string>;
 StartLocal?: (shell:string,cwd:string,cols:number,rows:number) => Promise<string>;
 GetDefaultShell?: NonNullable<NetcattyBridge['getDefaultShell']>;
 DiscoverShells?: NonNullable<NetcattyBridge['discoverShells']>;
 ValidatePath?: NonNullable<NetcattyBridge['validatePath']>;
}

export function createLocalShellBridge(bindings: NativeLocalShellBindings, attach: (alias:string|undefined,id:string)=>Promise<void>) {
 const required = <T>(method:T|undefined,name:string):T => {
  if (!method) throw new Error(`Native terminal method ${name} is unavailable`);
  return method;
 };
 return {
  startLocalSession: async (options:LocalOptions = {}) => {
   const request = {shell:options.shell ?? '',shellArgs:options.shellArgs ?? [],cwd:options.cwd ?? '',cols:options.cols ?? 80,rows:options.rows ?? 24,env:options.env ?? {}};
   let id: string;
   if (bindings.StartLocalWithOptions) id = await bindings.StartLocalWithOptions(request);
   else {
    if (request.shellArgs.length || Object.keys(request.env).length) throw new Error('Native terminal does not support local shell options');
    id = await required(bindings.StartLocal,'StartLocal')(request.shell,request.cwd,request.cols,request.rows);
   }
   await attach(options.sessionId,id);
   return id;
  },
  getDefaultShell: () => required(bindings.GetDefaultShell,'GetDefaultShell')(),
  discoverShells: () => required(bindings.DiscoverShells,'DiscoverShells')(),
  validatePath: (path:string,kind:'file'|'directory'|'any' = 'any') => required(bindings.ValidatePath,'ValidatePath')(path,kind),
 };
}
