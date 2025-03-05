<template>
  <div class="card bg-base-200 shadow-sm">
    <div class="card-body p-4">
      <div class="flex justify-between items-start">
        <h3 class="card-title text-sm">{{ fixture.name }}</h3>
        <button
          class="btn btn-ghost btn-sm btn-square"
          @click="$emit('close')"
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

      <div class="divider my-2"></div>

      <div class="space-y-2">
        <div class="flex justify-between items-center">
          <span class="text-xs text-base-content/70">Status</span>
          <div class="badge" :class="statusBadgeClass">
            {{ fixture.status }}
          </div>
        </div>
        <div class="flex justify-between items-center">
          <span class="text-xs text-base-content/70">Type</span>
          <span class="text-xs font-medium">{{ fixture.type }}</span>
        </div>
        <div class="flex justify-between items-center">
          <span class="text-xs text-base-content/70">Client</span>
          <span class="text-xs font-medium">{{ fixture.clientName }}</span>
        </div>
      </div>

      <div class="divider my-2"></div>

      <div class="card-actions justify-end">
        <button
          class="btn btn-sm btn-primary"
          :class="{ 'btn-disabled': fixture.status === 'running' }"
          @click="runFixture"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-4 w-4"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zM9.555 7.168A1 1 0 008 8v4a1 1 0 001.555.832l3-2a1 1 0 000-1.664l-3-2z"
              clip-rule="evenodd"
            />
          </svg>
          Run
        </button>
        <button
          class="btn btn-sm btn-error"
          :class="{ 'btn-disabled': fixture.status !== 'running' }"
          @click="stopFixture"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-4 w-4"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fill-rule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zM8 7a1 1 0 00-1 1v4a1 1 0 001 1h4a1 1 0 001-1V8a1 1 0 00-1-1H8z"
              clip-rule="evenodd"
            />
          </svg>
          Stop
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { defineProps, defineEmits, computed } from "vue";

const props = defineProps({
  fixture: {
    type: Object,
    required: true,
  },
});

defineEmits(["close"]);

const statusBadgeClass = computed(() => {
  switch (props.fixture.status) {
    case "running":
      return "badge-success";
    case "stopped":
      return "badge-error";
    case "idle":
      return "badge-ghost";
    default:
      return "badge-ghost";
  }
});

const runFixture = () => {
  // Implement fixture run logic
  console.log("Running fixture:", props.fixture.name);
};

const stopFixture = () => {
  // Implement fixture stop logic
  console.log("Stopping fixture:", props.fixture.name);
};
</script>
