import { defineStore } from "pinia";
import { ref } from "vue";
import axios from "axios";

export const useUploadProgressStore = defineStore("uploadProgress", () => {
  // State
  const records = ref([]);

  // Generate a random ID for each upload
  const randomID = () => {
    return "upload-" + Math.random().toString(36).substring(2, 15);
  };

  // Add a new record to the list
  const addRecord = (record) => {
    records.value.push(record);
  };

  // Remove a record from the list by ID
  const removeRecord = (id) => {
    const index = records.value.findIndex((r) => r.id === id);
    if (index !== -1) {
      records.value.splice(index, 1);
    }
  };

  // Upload a file
  // Args:
  //  uploadRequestPath: the upload HTTP request path
  //  file: the javascript File object
  //  dest: destination filename, set to undefined if sid is set
  //  sid: terminal session ID, used to identify upload destination
  //  done: callback to execute when upload is completed
  const upload = (uploadRequestPath, file, dest, sid, done) => {
    let query = "?";

    if (sid !== undefined) {
      query += "terminal_sid=" + sid;
    }

    if (dest !== undefined) {
      query += "&dest=" + dest;
    }

    const id = randomID();
    const formData = new FormData();
    formData.append("file", file);

    // Add record to the list
    addRecord({ filename: file.name, id: id });

    // Create an axios instance for the upload
    axios
      .post(uploadRequestPath + query, formData, {
        headers: {
          "Content-Type": "multipart/form-data",
        },
        onUploadProgress: (progressEvent) => {
          if (progressEvent.total) {
            const percentComplete = Math.round(
              (progressEvent.loaded * 100) / progressEvent.total,
            );
            const progressBar = document.getElementById(id);
            if (progressBar) {
              progressBar.style.width = percentComplete + "%";
              const percentElement = progressBar.querySelector(".percent");
              if (percentElement) {
                percentElement.textContent = percentComplete + "%";
              }
            }
          }
        },
      })
      .then(() => {
        const progressBar = document.getElementById(id);
        if (progressBar) {
          progressBar.style.width = "100%";
        }

        // Display the progress bar for 1 more second after completion
        setTimeout(() => {
          removeRecord(id);
        }, 1000);

        // Execute done callback if provided
        if (done) {
          done();
        }
      })
      .catch((error) => {
        let errorMessage = "Upload failed";

        // Extract error message from response if available
        if (error.response && error.response.data) {
          errorMessage = error.response.data.error || errorMessage;
        }

        addRecord({
          error: true,
          filename: file.name,
          id: id,
          message: errorMessage,
        });

        // Remove error message after a delay
        setTimeout(() => {
          removeRecord(id);
        }, 5000);
      });
  };

  return {
    records,
    upload,
    addRecord,
    removeRecord,
  };
});
