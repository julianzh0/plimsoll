package com.acme.tasks

import com.acme.tasks.data.TaskRepository
import com.acme.tasks.data.loadTasks
import com.acme.tasks.legacy.Clock
import kotlinx.coroutines.runBlocking

fun main() = runBlocking {
    val repository = TaskRepository(Clock())
    repository.save(loadTasks())
}
