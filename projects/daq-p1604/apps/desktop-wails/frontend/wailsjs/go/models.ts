export namespace backend {
	
	export class LogEvent {
	    level: string;
	    category: string;
	    deviceId?: string;
	    source: string;
	    message: string;
	    detail?: string;
	    timestamp: number;
	
	    static createFrom(source: any = {}) {
	        return new LogEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.category = source["category"];
	        this.deviceId = source["deviceId"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	        this.timestamp = source["timestamp"];
	    }
	}
	export class LogFileState {
	    active: boolean;
	    outputDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new LogFileState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.outputDir = source["outputDir"];
	    }
	}

}

export namespace core {
	
	export class ChannelConfig {
	    index: number;
	    name: string;
	    enabled: boolean;
	    unit: string;
	    color: string;
	    precision: number;
	    rangeMin?: number;
	    rangeMax?: number;
	
	    static createFrom(source: any = {}) {
	        return new ChannelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.unit = source["unit"];
	        this.color = source["color"];
	        this.precision = source["precision"];
	        this.rangeMin = source["rangeMin"];
	        this.rangeMax = source["rangeMax"];
	    }
	}
	export class P1604Config {
	    samplingRate: number;
	    unit: string;
	    autoConnect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new P1604Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.samplingRate = source["samplingRate"];
	        this.unit = source["unit"];
	        this.autoConnect = source["autoConnect"];
	    }
	}
	export class PressureProfile {
	    id: string;
	    name: string;
	    address: string;
	    port: number;
	    samplingRate: number;
	    channels: ChannelConfig[];
	    p1604Config: P1604Config;
	    createdAt?: number;
	
	    static createFrom(source: any = {}) {
	        return new PressureProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.samplingRate = source["samplingRate"];
	        this.channels = this.convertValues(source["channels"], ChannelConfig);
	        this.p1604Config = this.convertValues(source["p1604Config"], P1604Config);
	        this.createdAt = source["createdAt"];
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
	export class DeviceState {
	    profile: PressureProfile;
	    status: number;
	    statusText: string;
	    error: string;
	    connectedAt: number;
	    acquiringAt: number;
	    samplingRate: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], PressureProfile);
	        this.status = source["status"];
	        this.statusText = source["statusText"];
	        this.error = source["error"];
	        this.connectedAt = source["connectedAt"];
	        this.acquiringAt = source["acquiringAt"];
	        this.samplingRate = source["samplingRate"];
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
	
	
	export class RecordingSession {
	    id: string;
	    outputDir: string;
	    filePrefix: string;
	    startTimeMs: number;
	    snapshotCount: number;
	    status: number;
	
	    static createFrom(source: any = {}) {
	        return new RecordingSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.outputDir = source["outputDir"];
	        this.filePrefix = source["filePrefix"];
	        this.startTimeMs = source["startTimeMs"];
	        this.snapshotCount = source["snapshotCount"];
	        this.status = source["status"];
	    }
	}
	export class ScanResult {
	    id: string;
	    name: string;
	    address: string;
	    port: number;
	    macAddress?: string;
	    serialNumber?: string;
	    firmwareVersion?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.macAddress = source["macAddress"];
	        this.serialNumber = source["serialNumber"];
	        this.firmwareVersion = source["firmwareVersion"];
	    }
	}

}

