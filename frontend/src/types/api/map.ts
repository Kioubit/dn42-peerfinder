import {isRecord, isString, isUnknownArray} from "@/types/main.ts";

export type MapData = {
    type: 'FeatureCollection';
    features: unknown[];
};

export function isMapData(value: unknown): value is MapData {
    return (
        isRecord(value) &&
        isString(value.type) &&
        isUnknownArray(value.features)
    );
}