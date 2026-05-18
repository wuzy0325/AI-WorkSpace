export namespace ports {
	
	export class DeviceInfo {
	    id: string;
	    name: string;
	    channels: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.channels = source["channels"];
	    }
	}

}

export namespace usecase {
	
	export class RuntimeStats {
	    batchesEmitted: number;
	    droppedFrames: number;
	    uptimeMs: number;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.batchesEmitted = source["batchesEmitted"];
	        this.droppedFrames = source["droppedFrames"];
	        this.uptimeMs = source["uptimeMs"];
	    }
	}
	export class Status {
	    state: number;
	    sampleRateHz: number;
	    batchCount: number;
	    sampleCount: number;
	    latestValues: number[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.sampleRateHz = source["sampleRateHz"];
	        this.batchCount = source["batchCount"];
	        this.sampleCount = source["sampleCount"];
	        this.latestValues = source["latestValues"];
	    }
	}

}

