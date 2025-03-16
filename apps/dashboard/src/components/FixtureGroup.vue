<template>
  <div class="card bg-base-100 shadow">
    <div class="card-body">
      <div class="flex justify-between items-center mb-4">
        <h2 class="card-title text-lg">Fixtures</h2>
        <div class="flex gap-2">
          <button class="btn btn-sm btn-ghost" title="Refresh">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-4 w-4"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                fill-rule="evenodd"
                d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z"
                clip-rule="evenodd"
              />
            </svg>
          </button>
          <button class="btn btn-sm btn-ghost" title="Settings">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-4 w-4"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                fill-rule="evenodd"
                d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z"
                clip-rule="evenodd"
              />
            </svg>
          </button>
        </div>
      </div>

      <div
        class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4"
      >
        <fixture-widget
          v-for="fixture in fixtures"
          :key="fixture.id"
          :fixture="fixture"
          @close="removeFixture(fixture)"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { useClientStore } from "@/stores/clientStore";
import FixtureWidget from "./FixtureWidget.vue";
import { computed } from "vue";

defineProps({
  groupName: {
    type: String,
    required: true,
  },
});

const clientStore = useClientStore();
const fixtures = computed(() => Object.values(clientStore.fixtures.value));

const removeFixture = (fixture) => {
  clientStore.removeFixture(fixture.mid);
};
</script>

<style lang="scss" scoped>
.fixture-group {
  background-color: white;
  border-radius: 0.5rem;
  padding: 1rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);

  .group-title {
    margin-bottom: 1rem;
    color: #212529;
    font-weight: 600;
  }

  .fixture-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
  }
}
</style>
