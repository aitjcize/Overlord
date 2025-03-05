<template>
  <div
    class="upload-progress-bars panel panel-warning"
    :class="{
      'upload-progress-bars-hidden': uploadProgressStore.records.length === 0,
    }"
  >
    <div class="panel-heading">Upload Progress</div>
    <div class="panel-body upload-progress-panel-body">
      <div
        v-for="record in uploadProgressStore.records"
        :key="record.id"
        class="progress-item"
      >
        <div v-if="record.error" class="progress-error">
          <div class="alert alert-danger upload-alert">
            <button
              type="button"
              class="close"
              aria-label="Close"
              @click="uploadProgressStore.removeRecord(record.id)"
            >
              <span aria-hidden="true">&times;</span>
            </button>
            <b>{{ record.filename }}</b
            ><br />
            {{ record.message }}
          </div>
        </div>
        <div v-else class="progress">
          <div
            class="progress-bar"
            :id="record.id"
            role="progressbar"
            aria-valuenow="0"
            aria-valuemin="0"
            aria-valuemax="100"
            :style="{ width: '0%' }"
          >
            <span class="percent">0%</span> - {{ record.filename }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { useUploadProgressStore } from "@/stores/uploadProgressStore";

// Use the upload progress store instead of local state
const uploadProgressStore = useUploadProgressStore();

// Generate a random ID for each upload
const randomID = () => {
  return "upload-" + Math.random().toString(36).substring(2, 15);
};

// Upload a file
// Args:
//  uploadRequestPath: the upload HTTP request path
//  file: the javascript File object
//  dest: destination filename, set to undefined if sid is set
//  sid: terminal session ID, used to identify upload destination
//  done: callback to execute when upload is completed
const upload = (uploadRequestPath, file, dest, sid, done) => {
  // Use the store's upload method instead
  uploadProgressStore.upload(uploadRequestPath, file, dest, sid, done);
};

// Expose the upload method for backwards compatibility
defineExpose({
  upload,
});
</script>

<style scoped>
.upload-progress-bars {
  position: fixed;
  bottom: 20px;
  right: 20px;
  width: 500px;
  z-index: 1000;
  background-color: rgba(15, 23, 42, 0.95);
  border-radius: 0.5rem;
  box-shadow:
    0 10px 15px -3px rgba(0, 0, 0, 0.2),
    0 4px 6px -2px rgba(0, 0, 0, 0.1),
    0 0 0 1px rgba(16, 185, 129, 0.2);
  max-height: 500px;
  overflow-y: auto;
  color: #e5e7eb;
  border: 1px solid rgba(51, 65, 85, 0.5);
}

.upload-progress-bars-hidden {
  display: none;
}

.panel-heading {
  background-color: rgba(16, 185, 129, 0.2);
  color: #e5e7eb;
  padding: 10px 15px;
  border-top-left-radius: 0.5rem;
  border-top-right-radius: 0.5rem;
  font-weight: bold;
  border-bottom: 1px solid rgba(51, 65, 85, 0.5);
}

.panel-body {
  padding: 15px;
}

.progress {
  margin-bottom: 10px;
  height: 30px;
  border-radius: 0.25rem;
  background-color: rgba(51, 65, 85, 0.3);
  overflow: hidden;
}

.progress-bar {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  padding-left: 10px;
  height: 100%;
  background-color: rgba(16, 185, 129, 0.7);
  color: #e5e7eb;
  transition: width 0.3s ease;
}

.progress-error .alert {
  background-color: rgba(239, 68, 68, 0.2);
  color: #e5e7eb;
  border: 1px solid rgba(239, 68, 68, 0.5);
  padding: 10px;
  margin-bottom: 10px;
  border-radius: 0.25rem;
}

.progress-error .close {
  color: #e5e7eb;
  opacity: 0.7;
  background: none;
  border: none;
  float: right;
  font-size: 1.5rem;
  font-weight: 700;
  line-height: 1;
  padding: 0;
  cursor: pointer;
}

.progress-error .close:hover {
  opacity: 1;
}

@media (max-width: 768px) {
  .upload-progress-bars {
    width: calc(100% - 40px);
    left: 20px;
  }
}
</style>
