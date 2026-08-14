export namespace board {
	
	export class Command {
	    id: string;
	    name: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new Command(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
	    }
	}
	export class CommandList {
	    commands: Command[];
	    path: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new CommandList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commands = this.convertValues(source["commands"], Command);
	        this.path = source["path"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CommandFileResult {
	    list: CommandList;
	    path: string;
	    canceled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CommandFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.list = this.convertValues(source["list"], CommandList);
	        this.path = source["path"];
	        this.canceled = source["canceled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class CommandResult {
	    command: string;
	    stdout: string;
	    stderr: string;
	    success: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new CommandResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	        this.stdout = source["stdout"];
	        this.stderr = source["stderr"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class Device {
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    keyPath: string;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.keyPath = source["keyPath"];
	    }
	}
	export class Entry {
	    name: string;
	    size: number;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.isDir = source["isDir"];
	    }
	}
	export class Settings {
	    device: Device;
	    connectTimeoutSeconds: number;
	    commandTimeoutSeconds: number;
	    defaultPath: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = this.convertValues(source["device"], Device);
	        this.connectTimeoutSeconds = source["connectTimeoutSeconds"];
	        this.commandTimeoutSeconds = source["commandTimeoutSeconds"];
	        this.defaultPath = source["defaultPath"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Status {
	    connected: boolean;
	    addr: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.addr = source["addr"];
	        this.error = source["error"];
	    }
	}
	export class UploadResult {
	    remotePath: string;
	    needsConfirm: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UploadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.remotePath = source["remotePath"];
	        this.needsConfirm = source["needsConfirm"];
	    }
	}

}

export namespace netcfg {
	
	export class Config {
	    iface: string;
	    ip: string;
	    mask: string;
	    gateway: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.iface = source["iface"];
	        this.ip = source["ip"];
	        this.mask = source["mask"];
	        this.gateway = source["gateway"];
	    }
	}
	export class Device {
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	    }
	}
	export class Port {
	    name: string;
	    iface: string;
	    editable: boolean;
	    mac: string;
	    up: boolean;
	    ip: string;
	    mask: string;
	    gateway: string;
	
	    static createFrom(source: any = {}) {
	        return new Port(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.iface = source["iface"];
	        this.editable = source["editable"];
	        this.mac = source["mac"];
	        this.up = source["up"];
	        this.ip = source["ip"];
	        this.mask = source["mask"];
	        this.gateway = source["gateway"];
	    }
	}
	export class Settings {
	    device: Device;
	    mask: string;
	    gateway: string;
	    restoreFile: string;
	    connectTimeoutSeconds: number;
	    persistIface: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = this.convertValues(source["device"], Device);
	        this.mask = source["mask"];
	        this.gateway = source["gateway"];
	        this.restoreFile = source["restoreFile"];
	        this.connectTimeoutSeconds = source["connectTimeoutSeconds"];
	        this.persistIface = source["persistIface"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace remote {
	
	export class Device {
	    host: string;
	    port: number;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.path = source["path"];
	    }
	}
	export class DeviceSettings {
	    device: Device;
	    connectTimeoutSeconds: number;
	    requestTimeoutSeconds: number;
	    refreshIntervalMs: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = this.convertValues(source["device"], Device);
	        this.connectTimeoutSeconds = source["connectTimeoutSeconds"];
	        this.requestTimeoutSeconds = source["requestTimeoutSeconds"];
	        this.refreshIntervalMs = source["refreshIntervalMs"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FlowStep {
	    label: string;
	    type: string;
	    port: number;
	    action: string;
	    value: string;
	    onValue: number;
	    offValue: number;
	    pulseMs: number;
	    delayMs: number;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new FlowStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.type = source["type"];
	        this.port = source["port"];
	        this.action = source["action"];
	        this.value = source["value"];
	        this.onValue = source["onValue"];
	        this.offValue = source["offValue"];
	        this.pulseMs = source["pulseMs"];
	        this.delayMs = source["delayMs"];
	        this.hint = source["hint"];
	    }
	}
	export class Point {
	    label: string;
	    type: string;
	    port: number;
	    onValue: number;
	    offValue: number;
	    value: string;
	    pulseMs: number;
	    danger: boolean;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new Point(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.type = source["type"];
	        this.port = source["port"];
	        this.onValue = source["onValue"];
	        this.offValue = source["offValue"];
	        this.value = source["value"];
	        this.pulseMs = source["pulseMs"];
	        this.danger = source["danger"];
	        this.hint = source["hint"];
	    }
	}
	export class Group {
	    title: string;
	    points: Point[];
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.points = this.convertValues(source["points"], Point);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IOPoint {
	    type: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new IOPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.port = source["port"];
	    }
	}
	export class IOValue {
	    type: string;
	    port: number;
	    value: number;
	
	    static createFrom(source: any = {}) {
	        return new IOValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.port = source["port"];
	        this.value = source["value"];
	    }
	}
	export class Tab {
	    id: string;
	    title: string;
	    kind: string;
	    description: string;
	    groups?: Group[];
	    steps?: FlowStep[];
	
	    static createFrom(source: any = {}) {
	        return new Tab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.groups = this.convertValues(source["groups"], Group);
	        this.steps = this.convertValues(source["steps"], FlowStep);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    device: Device;
	    connectTimeoutSeconds: number;
	    requestTimeoutSeconds: number;
	    refreshIntervalMs: number;
	    tabs: Tab[];
	    configDir: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = this.convertValues(source["device"], Device);
	        this.connectTimeoutSeconds = source["connectTimeoutSeconds"];
	        this.requestTimeoutSeconds = source["requestTimeoutSeconds"];
	        this.refreshIntervalMs = source["refreshIntervalMs"];
	        this.tabs = this.convertValues(source["tabs"], Tab);
	        this.configDir = source["configDir"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PanelFileResult {
	    settings: Settings;
	    path: string;
	    canceled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PanelFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.path = source["path"];
	        this.canceled = source["canceled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RegisterValue {
	    address: number;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new RegisterValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.value = source["value"];
	    }
	}
	
	export class Status {
	    connected: boolean;
	    addr: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.addr = source["addr"];
	        this.error = source["error"];
	    }
	}

}

