export namespace backend {
	
	export class InterpolationResult {
	    alpha: number;
	    beta: number;
	    machNumber: number;
	    velocity: number;
	    cas: number;
	    sat: number;
	    dynamicPressure: number;
	    density: number;
	    P0: number;
	    Ps: number;
	    isValid: boolean;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new InterpolationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alpha = source["alpha"];
	        this.beta = source["beta"];
	        this.machNumber = source["machNumber"];
	        this.velocity = source["velocity"];
	        this.cas = source["cas"];
	        this.sat = source["sat"];
	        this.dynamicPressure = source["dynamicPressure"];
	        this.density = source["density"];
	        this.P0 = source["P0"];
	        this.Ps = source["Ps"];
	        this.isValid = source["isValid"];
	        this.warning = source["warning"];
	    }
	}
	export class BatchCalculateResponse {
	    success: boolean;
	    error?: string;
	    data?: InterpolationResult[];
	
	    static createFrom(source: any = {}) {
	        return new BatchCalculateResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	        this.data = this.convertValues(source["data"], InterpolationResult);
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
	export class CalculateResponse {
	    success: boolean;
	    error?: string;
	    data?: InterpolationResult;
	
	    static createFrom(source: any = {}) {
	        return new CalculateResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	        this.data = this.convertValues(source["data"], InterpolationResult);
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
	export class InterpolationInput {
	    P1: number;
	    P2: number;
	    P3: number;
	    P4: number;
	    P5: number;
	    Patm: number;
	    Tatm: number;
	    pressureMode: string;
	
	    static createFrom(source: any = {}) {
	        return new InterpolationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.P1 = source["P1"];
	        this.P2 = source["P2"];
	        this.P3 = source["P3"];
	        this.P4 = source["P4"];
	        this.P5 = source["P5"];
	        this.Patm = source["Patm"];
	        this.Tatm = source["Tatm"];
	        this.pressureMode = source["pressureMode"];
	    }
	}
	export class ImportCsvDataResponse {
	    success: boolean;
	    error?: string;
	    data?: InterpolationInput[];
	
	    static createFrom(source: any = {}) {
	        return new ImportCsvDataResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	        this.data = this.convertValues(source["data"], InterpolationInput);
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
	
	
	export class PrbValidRange {
	    alphaMin: number;
	    alphaMax: number;
	    betaMin: number;
	    betaMax: number;
	
	    static createFrom(source: any = {}) {
	        return new PrbValidRange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alphaMin = source["alphaMin"];
	        this.alphaMax = source["alphaMax"];
	        this.betaMin = source["betaMin"];
	        this.betaMax = source["betaMax"];
	    }
	}
	export class PrbFileInfo {
	    filePath: string;
	    fileName: string;
	    machNumber: number;
	    validRange: PrbValidRange;
	
	    static createFrom(source: any = {}) {
	        return new PrbFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filePath = source["filePath"];
	        this.fileName = source["fileName"];
	        this.machNumber = source["machNumber"];
	        this.validRange = this.convertValues(source["validRange"], PrbValidRange);
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
	export class LoadPrbResult {
	    files: PrbFileInfo[];
	    machRange: number[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new LoadPrbResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], PrbFileInfo);
	        this.machRange = source["machRange"];
	        this.warnings = source["warnings"];
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
	export class LoadPrbResponse {
	    success: boolean;
	    error?: string;
	    data?: LoadPrbResult;
	
	    static createFrom(source: any = {}) {
	        return new LoadPrbResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	        this.data = this.convertValues(source["data"], LoadPrbResult);
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
	
	export class MachRangeResponse {
	    success: boolean;
	    error?: string;
	    data?: number[];
	
	    static createFrom(source: any = {}) {
	        return new MachRangeResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	        this.data = source["data"];
	    }
	}
	

}

