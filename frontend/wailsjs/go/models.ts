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

