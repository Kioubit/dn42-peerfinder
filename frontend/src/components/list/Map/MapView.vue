<script setup lang="ts">
import {computed, onMounted, onUnmounted, reactive, ref, useTemplateRef} from 'vue'
import 'ol/ol.css'
import Map from 'ol/Map'
import View from 'ol/View'
import TileLayer from 'ol/layer/Tile'
import VectorLayer from 'ol/layer/Vector'
import VectorSource from 'ol/source/Vector'
import GeoJSON from 'ol/format/GeoJSON'
import {Circle as CircleStyle, Fill, RegularShape, Stroke, Style} from 'ol/style'
import Overlay from 'ol/Overlay'
import {fromLonLat} from 'ol/proj'
import {OSM} from "ol/source";
import type Geometry from "ol/geom/Geometry";
import {Feature} from "ol";
import type {Coordinate} from "ol/coordinate";
import {Point} from "ol/geom";
import type {Pixel} from "ol/pixel";
import {isMapData, type MapData} from "@/types/api/map.ts";

const emit = defineEmits(['select-locode'])

/* ── template refs ── */
const mapEl = useTemplateRef<HTMLDivElement>("mapEl");
const popupEl = useTemplateRef<HTMLDivElement>("popupEl");

const error = ref<string|null>(null)

/* ── reactive popup state ── */
interface PopupState {
  visible: boolean
  pinned: boolean
  kind: 'country' | 'city' | ''
  title: string
  subtitle: string
  count: number
  code: string
}

const popup = reactive<PopupState>({
  visible: false,
  pinned: false,
  kind: '',
  title: '',
  subtitle: '',
  count: 0,
  code: ''
})

let map: Map | null = null;
let overlay: Overlay | null = null;
let vectorSource: VectorSource<Feature<Geometry>> | null = null

/* ── styling ── */

/* ── dynamic tier thresholds (computed from data) ── */
const tierThresholds = ref<[number,number,number]>([Infinity, Infinity, Infinity])

function computeTierThresholds(features: Feature<Geometry>[]) {
  const counts = features
      .filter(f => f.get('kind') === 'city')
      .map(f => f.get('count') || 1)

  const unique = [...new Set(counts)].sort((a, b) => a - b)
  if (unique.length <= 1) {
    tierThresholds.value = [Infinity, Infinity, Infinity]
    return
  }
  if (unique.length === 2) {
    tierThresholds.value = [unique[1], Infinity, Infinity]
    return
  }
  if (unique.length === 3) {
    tierThresholds.value = [unique[1], unique[2], Infinity]
    return
  }

  const n = unique.length
  tierThresholds.value = [
    unique[Math.floor(n * 0.15)],   // green starts here
    unique[Math.floor(n * 0.45)],   // orange starts here
    unique[Math.floor(3 * n / 4)],  // red starts here
  ]
}

function tierColor(count: number): string {
  const [t1, t2, t3] = tierThresholds.value
  if (count >= t3) return '#dc3545'
  if (count >= t2) return '#fd7e14'
  if (count >= t1) return '#198754'
  return '#0d6efd'
}

const activeTier = ref<number|null>(null)  // null = show all

function tierIndex(count: number): number {
  const [t1, t2, t3] = tierThresholds.value
  if (count >= t3) return 3
  if (count >= t2) return 2
  if (count >= t1) return 1
  return 0
}

function applyTierFilter() {
  if (!vectorSource) return
  vectorSource.getFeatures().forEach(f => {
    if (f.get('kind') !== 'city') return
    const ti = tierIndex(f.get('count') || 1)
    const visible = activeTier.value === null || ti === activeTier.value
    f.setStyle(
        visible ? cityStyle(f.get('count') || 1) : undefined
    )
  })
}

function hitTest(coordinate: Coordinate): Feature<Geometry> | null {
  if (!vectorSource || !map) return null;

  const resolution = map.getView().getResolution();
  if (!resolution) return null;
  const tolerancePx = 8
  const tol = tolerancePx * resolution
  const tolSq = tol * tol

  // Check Point features (cities) first - they render on top
  for (const f of vectorSource.getFeatures()) {
    const geom = f.getGeometry()

    if (geom instanceof Point) {
      const c = geom.getCoordinates()
      if (c[0] === undefined || c[1] === undefined ||
          coordinate[0] === undefined || coordinate[1] === undefined) {
        continue
      }
      const dx = c[0] - coordinate[0]
      const dy = c[1] - coordinate[1]
      if (dx * dx + dy * dy <= tolSq) return f
    }
  }

  // Then check Polygon features (countries)
  for (const f of vectorSource.getFeatures()) {
    const geom = f.getGeometry()
    if (geom && geom.getType() !== 'Point' && geom.intersectsCoordinate(coordinate)) {
      return f
    }
  }

  return null
}

function cityStyle(count: number): Style {
  const radius = 6 + Math.sqrt(count) * 2
  return new Style({
    image: new CircleStyle({
      radius,
      fill: new Fill({ color: tierColor(count) }),
      stroke: new Stroke({ color: 'rgba(255,255,255,0.85)', width: 1.5 }),
    }),
  })
}

function countryStyle(hasCities: boolean): Style {
  const fill   = hasCities ? 'rgba(13,110,253,0.12)' : 'rgba(108,53,165,0.32)'
  const stroke = hasCities ? '#0d6efd' : '#6c35a5'
  return new Style({
    fill: new Fill({ color: fill }),
    stroke: new Stroke({ color: stroke, width: 1.5 }),
    image: new RegularShape({
      points: 4,
      radius: 10,
      angle: Math.PI / 4,
      fill: new Fill({ color: stroke }),
      stroke: new Stroke({ color: '#ff0000', width: 1.5 }),
    }),
  })
}

const legendTiers = computed(() => {
  const [t1, t2, t3] = tierThresholds.value
  return [
    { min: 1,  max: t1 - 1, color: '#0d6efd' },
    { min: t1, max: t2 - 1, color: '#198754' },
    { min: t2, max: t3 - 1, color: '#fd7e14' },
    { min: t3, max: Infinity, color: '#dc3545' },
  ]
      .filter(t => t.min <= t.max && t.min !== Infinity)
      .map(t => ({
        label: t.max === Infinity
            ? `${t.min}+`
            : t.min === t.max
                ? `${t.min}`
                : `${t.min}–${t.max}`,
        color: t.color,
      }))
})

/* ── popup helpers ── */
function fillPopup(feature: Feature<Geometry>) {
  const kind = feature.get('kind');
  const count = feature.get('count') || 0;

  if (kind === 'country') {
    const countryName = feature.get('countryName') || feature.get('country') || '';
    popup.kind = 'country';
    popup.title = countryName;
    popup.subtitle = feature.get('hasCities') ? 'has city servers' : 'country-level only';
    popup.count = count;
    popup.code = feature.get('country') || '';
  } else {
    popup.kind = 'city';
    popup.title = feature.get('locode') || '';
    popup.subtitle = feature.get('countryName') || feature.get('country') || '';
    popup.count = count;
    popup.code =  feature.get('locode') || '';
  }
}

function showPopup(feature: Feature<Geometry>, coordinate: Coordinate) {
  if (!overlay) return;

  fillPopup(feature);
  overlay.setPosition(coordinate);
  popup.visible = true;
}

function hidePopup() {
  if (overlay) {
    overlay.setPosition(undefined);
  }
  popup.visible = false;
  popup.pinned = false;
}

function closePopup() {
  hidePopup();
}

/* ── data fetch ── */
const fetchMapData = async (): Promise<MapData | null>  => {
  try {
    const res = await fetch(`api/directory/map_data`);
    if (!res.ok) throw new Error(`HTTP error! status: ${res.status}`);
    const data = await res.json();
    if (!isMapData(data)) {
      throw new Error("invalid data");
    }
    return data;
  } catch (e) {
    error.value = `Failed to fetch the map`;
    console.error('Error loading map:', e);
    return null;
  }
}
const data = await fetchMapData();
let rafId: number | null = null;

/* ── lifecycle ── */
onMounted(async () => {
  try {
    if (!data) {
      return;
    }
    error.value = null;

    vectorSource = new VectorSource({
      features: new GeoJSON().readFeatures(data, {
        dataProjection: 'EPSG:4326',
        featureProjection: 'EPSG:3857',
      }),
      wrapX: false,
    })

    computeTierThresholds(vectorSource.getFeatures());


    vectorSource.getFeatures().forEach((f) => {
      const kind = f.get('kind');
      const count = f.get('count') || 1;
      f.setStyle(kind === 'country' ? countryStyle(f.get('hasCities')) : cityStyle(count));
    })

    if (!popupEl.value || !mapEl.value) {
      throw new Error('Map elements are not mounted');
    }

    overlay = new Overlay({
      element: popupEl.value,
      positioning: 'bottom-center',
      stopEvent: true,
      autoPan: false,
      offset: [0, -12],
    })

    const min = fromLonLat([-180, -85.06]);
    const max = fromLonLat([180, 85.06]);

    map = new Map({
      target: mapEl.value,
      layers: [
        new TileLayer({ source: new OSM() }),
        /*
        new TileLayer({
          source: new XYZ({
            url: 'https://{a-c}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}.png',
            attributions: '© OpenStreetMap contributors © CARTO',
            wrapX: false
          }),
        }),
         */
        new VectorLayer({ source: vectorSource }),
      ],
      overlays: [overlay],
      view: new View({
        center: fromLonLat([0, 40]), minZoom: 2, zoom: 2, multiWorld: false,
        extent: [
          min[0]!,
          min[1]!,
          max[0]!,
          max[1]!,
        ],
      }),
    })

    // hover
    let lastPixel: Pixel | null = null;

    map.on('pointermove', (evt) => {
      if (evt.dragging) return;

      lastPixel = evt.pixel;

      if (rafId) return;

      rafId = requestAnimationFrame(() => {
        rafId = null;

        const pixel = lastPixel;
        if (!map || !pixel) return;

        const coordinate = map.getCoordinateFromPixel(pixel);
        const feature = hitTest(coordinate);

        if (feature) {
          if (!popup.pinned) {
            showPopup(feature, coordinate);
          }

          if (mapEl.value) {
            mapEl.value.style.cursor = 'pointer';
          }
        } else {
          if (!popup.pinned) {
            hidePopup();
          }

          if (mapEl.value) {
            mapEl.value.style.cursor = '';
          }
        }
      });
    });

    map.on('singleclick', (evt) => {
      const coordinate = evt.coordinate;
      const feature = hitTest(coordinate);
      if (feature) {
        popup.pinned = true;
        showPopup(feature, evt.coordinate);
      } else {
        hidePopup();
      }
    })
  } catch (e) {
    error.value = `Failed to load map: ${
        e instanceof Error ? e.message : String(e)
    }`;
    console.error('Error loading map:', e);
  }
})

onUnmounted(() => {
  if (rafId) cancelAnimationFrame(rafId);
  if (map) {
    map.setTarget(undefined);
    map = null;
    overlay = null;
  }
})
</script>

<template>
  <div class="card border-0 shadow-sm rounded-3 mb-4 overflow-hidden">
    <div class="card-header bg-white border-bottom d-flex align-items-center justify-content-between py-3 flex-wrap gap-2">
      <div>
        <h5 class="fw-bold mb-0">Server Location Map</h5>
      </div>
      <div class="d-flex align-items-center gap-3 flex-wrap" v-if="!error">
        <span
            v-for="(t, i) in legendTiers"
            :key="t.label"
            class="d-flex align-items-center gap-1 small"
            :class="activeTier === null || activeTier === i ? 'text-muted' : 'text-muted opacity-50'"
            role="button"
            style="cursor: pointer;"
            @click="activeTier = activeTier === i ? null : i; applyTierFilter()"
        >
          <span class="rounded-circle d-inline-block" :style="{ width: '12px', height: '12px', background: t.color }"></span>
          {{ t.label }}
        </span>
                <span
                    v-if="activeTier !== null"
                    role="button"
                    class="small text-primary text-decoration-underline ms-1"
                    @click="activeTier = null; applyTierFilter()"
                >
          show all
        </span>
        <span class="d-flex align-items-center gap-1 small text-muted ms-2">
          <span class="d-inline-block" :style="{ width: '14px', height: '12px', background: 'rgba(108,53,165,0.32)', border: '1.5px solid #6c35a5' }"></span>
          country
        </span>
        <span class="d-flex align-items-center gap-1 small text-muted">
          <span class="d-inline-block" :style="{ width: '14px', height: '12px', background: 'rgba(13,110,253,0.12)', border: '1.5px solid #0d6efd' }"></span>
          + cities
        </span>
      </div>
    </div>

    <div style="position: relative;">
      <!-- Error -->
      <div v-if="error" class="alert alert-warning m-3 rounded-3 border-0" role="alert">
        {{ error }}
      </div>

      <!-- Map -->
      <div v-show="!error" ref="mapEl" style="height: 420px; width: 100%;"></div>

      <div ref="popupEl" class="ol-popup">
        <div v-if="popup.visible" class="card border-0 shadow rounded-3 px-3 py-2" style="min-width: 160px;">
          <button
              type="button"
              class="btn-close btn-sm position-absolute top-0 end-0 m-1"
              style="font-size: .6rem;"
              aria-label="Close"
              @click="closePopup"
          ></button>
          <div class="fw-bold">{{ popup.title }}</div>
          <div v-if="popup.subtitle" class="small text-muted">{{ popup.subtitle }}</div>
          <div class="small mt-1">
            <span class="badge bg-primary">{{ popup.count }}</span>
            server{{ popup.count === 1 ? '' : 's' }}
          </div>
          <button
              type="button"
              class="btn btn-sm btn-outline-primary mt-1"
              @click="emit('select-locode', popup.code)"
          >
            View servers
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>

</style>