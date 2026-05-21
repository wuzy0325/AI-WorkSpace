export namespace backend {
	
	export class VersionInfo {
	    name: string;
	    version: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.port = source["port"];
	    }
	}

}

