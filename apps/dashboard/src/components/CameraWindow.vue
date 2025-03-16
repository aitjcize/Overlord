<template>
  <div
    class="card bg-base-200 shadow-lg h-[400px] flex flex-col fixed inset-0 w-full h-full lg:static lg:inset-auto"
    :class="{ 'lg:h-[400px]': !isFullscreen }"
  >
    <div class="window-header">
      <div class="flex flex-col">
        <h3 class="window-title">{{ camera.clientName }}</h3>
        <span class="window-subtitle">Camera: {{ camera.id }}</span>
      </div>
      <div class="flex gap-2">
        <button
          class="btn btn-ghost btn-sm btn-square"
          @click="toggleFullscreen"
          title="Toggle Fullscreen"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-4 w-4"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              v-if="isFullscreen"
              fill-rule="evenodd"
              d="M5 4a1 1 0 0 1 1-1h4a1 1 0 1 1 0 2H6.414l4.293 4.293a1 1 0 0 1-1.414 1.414L5 6.414V10a1 1 0 1 1-2 0V5a1 1 0 0 1 1-1zm10 0a1 1 0 0 1 1 1v5a1 1 0 1 1-2 0V6.414l-4.293 4.293a1 1 0 1 1-1.414-1.414L13.586 5H10a1 1 0 1 1 0-2h5a1 1 0 0 1 1 1z"
              clip-rule="evenodd"
            />
            <path
              v-else
              fill-rule="evenodd"
              d="M3 4a1 1 0 0 1 1-1h4a1 1 0 0 1 0 2H4v4a1 1 0 0 1-2 0V4zm12-1a1 1 0 0 1 1 1v4a1 1 0 1 1-2 0V4h-4a1 1 0 1 1 0-2h5zM3 16a1 1 0 0 1 1 1v.5a.5.5 0 0 0 .5.5H8a1 1 0 1 1 0 2H4.5A2.5 2.5 0 0 1 2 17.5V16a1 1 0 0 1 1-1zm12 0a1 1 0 0 1 1 1v1.5a2.5 2.5 0 0 1-2.5 2.5H12a1 1 0 1 1 0-2h3.5a.5.5 0 0 0 .5-.5V16a1 1 0 0 1 1-1z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
        <button
          class="btn btn-ghost btn-sm btn-square"
          @click="closeCamera"
          title="Close"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-4 w-4"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
      </div>
    </div>
    <div
      class="flex-1 bg-neutral overflow-hidden relative"
      ref="videoContainer"
      :class="{ 'fixed inset-0 z-50': isFullscreen }"
    >
      <video
        ref="videoElement"
        class="w-full h-full object-contain"
        autoplay
        playsinline
      ></video>
      <div
        v-if="!isConnected"
        class="absolute inset-0 flex flex-col items-center justify-center bg-neutral/70 gap-4"
      >
        <span class="loading loading-spinner loading-lg text-primary"></span>
        <span class="text-base-content">Connecting to camera...</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount } from "vue";
import { useClientStore } from "@/stores/clientStore";

const clientStore = useClientStore();

const props = defineProps({
  camera: {
    type: Object,
    required: true,
  },
});

const videoContainer = ref(null);
const videoElement = ref(null);
const isFullscreen = ref(false);
const isConnected = ref(false);
let ws = null;

// Define handlers at the top of the script
const handleCameraResize = () => {
  if (videoContainer.value) {
    const containerWidth = videoContainer.value.clientWidth;
    const containerHeight = videoContainer.value.clientHeight;

    // Update video dimensions based on container size
    if (videoElement.value) {
      videoElement.value.style.width = `${containerWidth}px`;
      videoElement.value.style.height = `${containerHeight}px`;
    }
  }
};

const handleFullscreenChange = () => {
  isFullscreen.value = document.fullscreenElement === videoContainer.value;
  handleCameraResize();
};

onMounted(() => {
  // Connect to WebSocket for camera stream
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  ws = new WebSocket(
    `${protocol}//${window.location.host}/api/camera/${props.camera.id}`,
  );

  ws.onopen = () => {
    isConnected.value = true;
  };

  ws.onmessage = async (event) => {
    try {
      const blob =
        event.data instanceof Blob ? event.data : new Blob([event.data]);
      const url = URL.createObjectURL(blob);
      videoElement.value.src = url;
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error("Error processing camera frame:", error);
    }
  };

  ws.onclose = () => {
    isConnected.value = false;
  };

  // Add event listener for fullscreen changes
  document.addEventListener("fullscreenchange", handleFullscreenChange);
});

onBeforeUnmount(() => {
  // Clean up
  if (ws) {
    ws.close();
  }
  document.removeEventListener("fullscreenchange", handleFullscreenChange);
});

const toggleFullscreen = async () => {
  try {
    if (!document.fullscreenElement) {
      await videoContainer.value.requestFullscreen();
    } else {
      await document.exitFullscreen();
    }
  } catch (error) {
    console.error("Error toggling fullscreen:", error);
  }
};

const closeCamera = () => {
  clientStore.removeCamera(props.camera.id);
};
</script>

<style lang="scss" scoped>
.card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid #d1d5db;
  border-radius: 0.5rem;
  box-shadow:
    0 4px 6px -1px rgba(0, 0, 0, 0.1),
    0 2px 4px -1px rgba(0, 0, 0, 0.05);
  will-change: transform;
  backface-visibility: hidden;
  transform: translateZ(0);
  background-color: #e5e7eb;
  opacity: 0.6;
  transition:
    opacity 0.2s ease,
    box-shadow 0.2s ease;
  z-index: 10;

  @media (min-width: 1024px) {
    position: relative;
    width: 600px;
    height: 400px;
    margin: 1rem;
  }
}

.camera-window {
  background-color: white;
  border-radius: 0.5rem;
  overflow: hidden;
  border: 1px solid #dee2e6;
  height: 400px;
  display: flex;
  flex-direction: column;

  .window-header {
    padding: 0.75rem;
    background-color: #e9ecef;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid #dee2e6;

    .client-info {
      display: flex;
      flex-direction: column;

      .client-name {
        font-weight: 600;
        font-size: 0.875rem;
      }

      .camera-id {
        font-size: 0.75rem;
        color: #6c757d;
      }
    }

    .camera-controls {
      display: flex;
      gap: 0.5rem;
      align-items: center;

      .btn {
        padding: 0.25rem 0.5rem;
      }

      .btn-close {
        padding: 0.25rem;
        font-size: 0.75rem;
      }
    }
  }

  .video-container {
    flex: 1;
    background-color: #000;
    position: relative;

    video {
      width: 100%;
      height: 100%;
      object-fit: contain;
    }

    &:fullscreen {
      padding: 0;
      width: 100vw;
      height: 100vh;

      video {
        object-fit: contain;
      }
    }

    .connection-overlay {
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      bottom: 0;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      background-color: rgba(0, 0, 0, 0.7);
      gap: 1rem;

      .status-text {
        color: white;
        font-size: 0.875rem;
      }
    }
  }
}
</style>
