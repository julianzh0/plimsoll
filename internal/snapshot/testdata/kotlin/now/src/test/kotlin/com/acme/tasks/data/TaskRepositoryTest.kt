package com.acme.tasks.data

import com.acme.tasks.legacy.Clock
import kotlin.test.Test

class TaskRepositoryTest {
    @Test fun saves() {
        TaskRepository(Clock()).save(emptyList())
    }
}
