export namespace backend {
	
	export class GenericResponse {
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GenericResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class VersionInfo {
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}

}

export namespace calibration {
	
	export class CalPoint {
	    id: number;
	    coordinates: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new CalPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.coordinates = source["coordinates"];
	    }
	}
	export class ProbeChannel {
	    role: string;
	    name: string;
	    deviceId: string;
	    channelIndex: number;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProbeChannel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.name = source["name"];
	        this.deviceId = source["deviceId"];
	        this.channelIndex = source["channelIndex"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Config {
	    taskId: string;
	    deviceId: string;
	    type: string;
	    channels: number[];
	    pressurePoints: number[];
	    averageSamples: number;
	    probeChannels?: ProbeChannel[];
	    points?: CalPoint[];
	    samplesPerPoint?: number;
	    dwellTimeMs?: number;
	    stopOnError?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.deviceId = source["deviceId"];
	        this.type = source["type"];
	        this.channels = source["channels"];
	        this.pressurePoints = source["pressurePoints"];
	        this.averageSamples = source["averageSamples"];
	        this.probeChannels = this.convertValues(source["probeChannels"], ProbeChannel);
	        this.points = this.convertValues(source["points"], CalPoint);
	        this.samplesPerPoint = source["samplesPerPoint"];
	        this.dwellTimeMs = source["dwellTimeMs"];
	        this.stopOnError = source["stopOnError"];
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
	export class PointResult {
	    pointIndex: number;
	    targetPressure: number;
	    timestamp: number;
	    values: Record<number, number>;
	
	    static createFrom(source: any = {}) {
	        return new PointResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pointIndex = source["pointIndex"];
	        this.targetPressure = source["targetPressure"];
	        this.timestamp = source["timestamp"];
	        this.values = source["values"];
	    }
	}
	
	export class Status {
	    taskId: string;
	    state: string;
	    currentPoint: number;
	    totalPoints: number;
	    results: PointResult[];
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.state = source["state"];
	        this.currentPoint = source["currentPoint"];
	        this.totalPoints = source["totalPoints"];
	        this.results = this.convertValues(source["results"], PointResult);
	        this.lastError = source["lastError"];
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

export namespace device {
	
	export class ChannelConfig {
	    index: number;
	    name: string;
	    enabled: boolean;
	    unit: string;
	    precision: number;
	    rangeMin?: number;
	    rangeMax?: number;
	    tareOffset?: number;
	
	    static createFrom(source: any = {}) {
	        return new ChannelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.unit = source["unit"];
	        this.precision = source["precision"];
	        this.rangeMin = source["rangeMin"];
	        this.rangeMax = source["rangeMax"];
	        this.tareOffset = source["tareOffset"];
	    }
	}
	export class DaqT1603HardwareConfig {
	    thermocoupleTypes: string;
	    channelMask: string;
	    samplingRate: number;
	    binaryFormat: boolean;
	    averageCount: number;
	    triggerMode: number;
	    triggerEdge: number;
	    triggerCount: number;
	    showTimestamp: boolean;
	    showSequence: boolean;
	    openCircuitCheck: string;
	
	    static createFrom(source: any = {}) {
	        return new DaqT1603HardwareConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.thermocoupleTypes = source["thermocoupleTypes"];
	        this.channelMask = source["channelMask"];
	        this.samplingRate = source["samplingRate"];
	        this.binaryFormat = source["binaryFormat"];
	        this.averageCount = source["averageCount"];
	        this.triggerMode = source["triggerMode"];
	        this.triggerEdge = source["triggerEdge"];
	        this.triggerCount = source["triggerCount"];
	        this.showTimestamp = source["showTimestamp"];
	        this.showSequence = source["showSequence"];
	        this.openCircuitCheck = source["openCircuitCheck"];
	    }
	}
	export class DataPayload {
	    deviceId: string;
	    timestamp: number;
	    channels: number[];
	    channelIndices: number[];
	
	    static createFrom(source: any = {}) {
	        return new DataPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deviceId = source["deviceId"];
	        this.timestamp = source["timestamp"];
	        this.channels = source["channels"];
	        this.channelIndices = source["channelIndices"];
	    }
	}
	export class Profile {
	    id: string;
	    name: string;
	    type: string;
	    transport?: string;
	    address?: string;
	    port?: number;
	    serialPort?: string;
	    baudRate?: number;
	    autoConnect?: boolean;
	    macAddress?: string;
	    samplingRate: number;
	    channels: ChannelConfig[];
	    daqT1603Config?: DaqT1603HardwareConfig;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.transport = source["transport"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.serialPort = source["serialPort"];
	        this.baudRate = source["baudRate"];
	        this.autoConnect = source["autoConnect"];
	        this.macAddress = source["macAddress"];
	        this.samplingRate = source["samplingRate"];
	        this.channels = this.convertValues(source["channels"], ChannelConfig);
	        this.daqT1603Config = this.convertValues(source["daqT1603Config"], DaqT1603HardwareConfig);
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
	export class ScanResult {
	    id: string;
	    name: string;
	    type: string;
	    available: boolean;
	    address?: string;
	    port?: number;
	    macAddress?: string;
	    serialNumber?: string;
	    firmwareVersion?: string;
	    model?: string;
	    subnetMask?: string;
	    gateway?: string;
	    ipMode?: string;
	    tcpConnected?: boolean;
	    ipAssigned?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.available = source["available"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.macAddress = source["macAddress"];
	        this.serialNumber = source["serialNumber"];
	        this.firmwareVersion = source["firmwareVersion"];
	        this.model = source["model"];
	        this.subnetMask = source["subnetMask"];
	        this.gateway = source["gateway"];
	        this.ipMode = source["ipMode"];
	        this.tcpConnected = source["tcpConnected"];
	        this.ipAssigned = source["ipAssigned"];
	    }
	}
	export class Status {
	    id: string;
	    name: string;
	    type: string;
	    connection: string;
	    acquiring: boolean;
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.connection = source["connection"];
	        this.acquiring = source["acquiring"];
	        this.lastError = source["lastError"];
	    }
	}

}

export namespace frontend {
	
	export class FileFilter {
	    DisplayName: string;
	    Pattern: string;
	
	    static createFrom(source: any = {}) {
	        return new FileFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.DisplayName = source["DisplayName"];
	        this.Pattern = source["Pattern"];
	    }
	}

}

export namespace motion {
	
	export class AxisConfig {
	    name: string;
	    enabled: boolean;
	    kind: string;
	    maxSpeed?: number;
	    minLimit?: number;
	    maxLimit?: number;
	    inverted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AxisConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.kind = source["kind"];
	        this.maxSpeed = source["maxSpeed"];
	        this.minLimit = source["minLimit"];
	        this.maxLimit = source["maxLimit"];
	        this.inverted = source["inverted"];
	    }
	}
	export class AxisStatus {
	    name: string;
	    position: number;
	    velocity: number;
	    moving: boolean;
	    homed: boolean;
	    posLimit: boolean;
	    negLimit: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AxisStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.position = source["position"];
	        this.velocity = source["velocity"];
	        this.moving = source["moving"];
	        this.homed = source["homed"];
	        this.posLimit = source["posLimit"];
	        this.negLimit = source["negLimit"];
	    }
	}
	export class ControllerStatus {
	    id: string;
	    name: string;
	    type: string;
	    connected: boolean;
	    emergencyStopped: boolean;
	    axes: AxisStatus[];
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ControllerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.connected = source["connected"];
	        this.emergencyStopped = source["emergencyStopped"];
	        this.axes = this.convertValues(source["axes"], AxisStatus);
	        this.lastError = source["lastError"];
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
	export class MotionControllerProfile {
	    id: string;
	    name: string;
	    type: string;
	    address: string;
	    port: number;
	    autoConnect: boolean;
	    axes: AxisConfig[];
	
	    static createFrom(source: any = {}) {
	        return new MotionControllerProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.autoConnect = source["autoConnect"];
	        this.axes = this.convertValues(source["axes"], AxisConfig);
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

export namespace report {
	
	export class ReportStatus {
	    generating: boolean;
	    lastResult?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generating = source["generating"];
	        this.lastResult = source["lastResult"];
	    }
	}

}

export namespace storage {
	
	export class RecordingStatus {
	    recording: boolean;
	    outputDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new RecordingStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.recording = source["recording"];
	        this.outputDir = source["outputDir"];
	    }
	}

}

