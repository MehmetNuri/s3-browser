export namespace main {
	
	export class AppInfo {
	    name: string;
	    version: string;
	    copyright: string;
	    goVersion: string;
	    electronVersion: string;
	    platform: string;
	    configDir: string;
	    secretStorage: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.copyright = source["copyright"];
	        this.goVersion = source["goVersion"];
	        this.electronVersion = source["electronVersion"];
	        this.platform = source["platform"];
	        this.configDir = source["configDir"];
	        this.secretStorage = source["secretStorage"];
	    }
	}
	export class Bucket {
	    name: string;
	    created: string;
	    pinned: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Bucket(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.created = source["created"];
	        this.pinned = source["pinned"];
	    }
	}
	export class BucketList {
	    buckets: Bucket[];
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new BucketList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.buckets = this.convertValues(source["buckets"], Bucket);
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
	export class CapOptions {
	    bucket: string;
	    includeBucket: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CapOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bucket = source["bucket"];
	        this.includeBucket = source["includeBucket"];
	    }
	}
	export class CapResult {
	    id: string;
	    category: string;
	    name: string;
	    status: string;
	    detail: string;
	    ms: number;
	
	    static createFrom(source: any = {}) {
	        return new CapResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.category = source["category"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	        this.ms = source["ms"];
	    }
	}
	export class S3Object {
	    key: string;
	    name: string;
	    size: number;
	    modified: string;
	    etag: string;
	    storageClass: string;
	    isFolder: boolean;
	
	    static createFrom(source: any = {}) {
	        return new S3Object(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.modified = source["modified"];
	        this.etag = source["etag"];
	        this.storageClass = source["storageClass"];
	        this.isFolder = source["isFolder"];
	    }
	}
	export class Listing {
	    items: S3Object[];
	    nextToken: string;
	
	    static createFrom(source: any = {}) {
	        return new Listing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], S3Object);
	        this.nextToken = source["nextToken"];
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
	export class ObjectInfo {
	    key: string;
	    size: number;
	    contentType: string;
	    etag: string;
	    modified: string;
	    storageClass: string;
	    cacheControl: string;
	    contentDisposition: string;
	    contentEncoding: string;
	    versionId: string;
	    metadata: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ObjectInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.size = source["size"];
	        this.contentType = source["contentType"];
	        this.etag = source["etag"];
	        this.modified = source["modified"];
	        this.storageClass = source["storageClass"];
	        this.cacheControl = source["cacheControl"];
	        this.contentDisposition = source["contentDisposition"];
	        this.contentEncoding = source["contentEncoding"];
	        this.versionId = source["versionId"];
	        this.metadata = source["metadata"];
	    }
	}
	export class Profile {
	    id: string;
	    name: string;
	    provider: string;
	    endpoint: string;
	    region: string;
	    accessKey: string;
	    secretKey: string;
	    sessionToken: string;
	    pathStyle: boolean;
	    skipTLS: boolean;
	    projectRef: string;
	    buckets: string[];
	    storageClass: string;
	    encryption: string;
	    kmsKey: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.endpoint = source["endpoint"];
	        this.region = source["region"];
	        this.accessKey = source["accessKey"];
	        this.secretKey = source["secretKey"];
	        this.sessionToken = source["sessionToken"];
	        this.pathStyle = source["pathStyle"];
	        this.skipTLS = source["skipTLS"];
	        this.projectRef = source["projectRef"];
	        this.buckets = source["buckets"];
	        this.storageClass = source["storageClass"] ?? '';
	        this.encryption = source["encryption"] ?? '';
	        this.kmsKey = source["kmsKey"] ?? '';
	    }
	}

}

