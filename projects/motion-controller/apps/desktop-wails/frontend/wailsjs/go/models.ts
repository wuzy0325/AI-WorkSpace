export namespace core {
	
	export class AxisEncoderCompensationConfig {
	    enabled: boolean;
	    tolerance: number;
	    maxCycles: number;
	    settleMs: number;
	    minStep: number;
	    timeoutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new AxisEncoderCompensationConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.tolerance = source["tolerance"];
	        this.maxCycles = source["maxCycles"];
	        this.settleMs = source["settleMs"];
	        this.minStep = source["minStep"];
	        this.timeoutMs = source["timeoutMs"];
	    }
	}
	export class AxisConfig {
	    name: string;
	    enabled: boolean;
	    kind: string;
	    stepsPerRev?: number;
	    microSteps?: number;
	    lead?: number;
	    gearRatio?: number;
	    maxSpeed?: number;
	    minLimit?: number;
	    maxLimit?: number;
	    inverted: boolean;
	    encoderInverted: boolean;
	    positionSource: string;
	    encoderScale?: number;
	    encoderCompensation?: AxisEncoderCompensationConfig;
	
	    static createFrom(source: any = {}) {
	        return new AxisConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.kind = source["kind"];
	        this.stepsPerRev = source["stepsPerRev"];
	        this.microSteps = source["microSteps"];
	        this.lead = source["lead"];
	        this.gearRatio = source["gearRatio"];
	        this.maxSpeed = source["maxSpeed"];
	        this.minLimit = source["minLimit"];
	        this.maxLimit = source["maxLimit"];
	        this.inverted = source["inverted"];
	        this.encoderInverted = source["encoderInverted"];
	        this.positionSource = source["positionSource"];
	        this.encoderScale = source["encoderScale"];
	        this.encoderCompensation = this.convertValues(source["encoderCompensation"], AxisEncoderCompensationConfig);
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

